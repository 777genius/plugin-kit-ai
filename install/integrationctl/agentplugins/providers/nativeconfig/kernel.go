package nativeconfig

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tailscale/hujson"
)

type resolvedFile struct {
	path   string
	body   []byte
	mode   os.FileMode
	exists bool
	jsonc  bool
}

func (kernel Kernel) Apply(req Request) (Receipt, error) {
	receipts, err := kernel.ApplyBatch([]Request{req})
	if err != nil {
		return Receipt{}, err
	}
	if len(receipts) == 0 {
		return Receipt{}, nil
	}
	return receipts[0], nil
}

// ApplyBatch validates and applies related MCP entry mutations through one
// compare-and-swap write. Either every requested entry changes or none do.
func (kernel Kernel) ApplyBatch(requests []Request) ([]Receipt, error) {
	if kernel.files == nil {
		return nil, fmt.Errorf("native config file IO is required")
	}
	if len(requests) == 0 {
		return nil, nil
	}
	for _, req := range requests {
		if err := validateRequest(req); err != nil {
			return nil, err
		}
		if req.Paths != requests[0].Paths || req.Codec != requests[0].Codec {
			return nil, fmt.Errorf("batched native config requests must share paths and codec")
		}
	}
	file, err := kernel.resolve(requests[0].Paths)
	if err != nil {
		return nil, err
	}
	doc, err := newDocument(file.jsonc)
	if file.exists {
		doc, err = parseDocument(file.body, file.jsonc)
	}
	if err != nil {
		return nil, err
	}
	key := codecCollectionKey(requests[0].Codec)
	create := false
	for _, req := range requests {
		create = create || req.Action == ActionAdd
	}
	entries, err := collection(doc, key, create)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for _, req := range requests {
		if _, duplicate := seen[req.Name]; duplicate {
			return nil, fmt.Errorf("duplicate native MCP batch entry %q", req.Name)
		}
		seen[req.Name] = struct{}{}
		var current *hujson.ObjectMember
		if entries != nil {
			current, _ = objectMember(entries, req.Name)
		}
		switch req.Action {
		case ActionAdd:
			if current != nil {
				return nil, fmt.Errorf("%w: %s", ErrCollision, req.Name)
			}
			projected, err := projectServer(req.Codec, req.Server, req.Placeholders)
			if err != nil {
				return nil, err
			}
			value, err := jsonValue(projected)
			if err != nil {
				return nil, err
			}
			setEntry(entries, req.Name, value)
		case ActionUpdate:
			if current == nil || verifyReceipt(req.Owned, file.path, req.Codec, req.Name, current) != nil {
				return nil, fmt.Errorf("%w: %s", ErrNotOwned, req.Name)
			}
			projected, err := projectServer(req.Codec, req.Server, req.Placeholders)
			if err != nil {
				return nil, err
			}
			value, err := jsonValue(projected)
			if err != nil {
				return nil, err
			}
			setEntry(entries, req.Name, value)
		case ActionRemove:
			if current == nil || verifyReceipt(req.Owned, file.path, req.Codec, req.Name, current) != nil {
				return nil, fmt.Errorf("%w: %s", ErrNotOwned, req.Name)
			}
			removeEntry(entries, req.Name)
		}
	}

	receipts := make([]Receipt, len(requests))
	for index, req := range requests {
		if req.Action == ActionRemove {
			continue
		}
		desired, err := DesiredReceipt(file.path, req.Codec, req.Name, req.Server, req.Placeholders)
		if err != nil {
			return nil, err
		}
		member, _ := objectMember(entries, req.Name)
		actualDigest, err := entryDigest(req.Codec, req.Name, member)
		if err != nil {
			return nil, err
		}
		if !strings.EqualFold(actualDigest, desired.Digest) {
			return nil, fmt.Errorf("native config projection differs from desired receipt")
		}
		receipts[index] = desired
	}
	next, err := doc.render()
	if err != nil {
		return nil, err
	}
	if err := kernel.writeVerified(file, next); err != nil {
		return nil, err
	}
	return receipts, nil
}

// Inspect performs a strict read-only ownership check for one native entry.
func (kernel Kernel) Inspect(paths Paths, codec Codec, name string, owned *Receipt) (present bool, exactlyOwned bool, err error) {
	if kernel.files == nil {
		return false, false, fmt.Errorf("native config file IO is required")
	}
	if strings.TrimSpace(name) == "" {
		return false, false, fmt.Errorf("MCP entry name is required")
	}
	probe := Request{Paths: paths, Codec: codec, Action: ActionAdd, Name: name}
	if err := validateRequest(probe); err != nil {
		return false, false, err
	}
	file, err := kernel.resolve(paths)
	if err != nil || !file.exists {
		return false, false, err
	}
	doc, err := parseDocument(file.body, file.jsonc)
	if err != nil {
		return false, false, err
	}
	key := codecCollectionKey(codec)
	entries, err := collection(doc, key, false)
	if err != nil || entries == nil {
		return false, false, err
	}
	member, _ := objectMember(entries, name)
	if member == nil {
		return false, false, nil
	}
	return true, verifyReceipt(owned, file.path, codec, name, member) == nil, nil
}

func (kernel Kernel) resolve(paths Paths) (resolvedFile, error) {
	jsonPath := filepath.Clean(paths.JSON)
	jsonBody, jsonMode, jsonExists, err := kernel.files.ReadNoFollow(jsonPath)
	if err != nil {
		return resolvedFile{}, err
	}
	var jsoncBody []byte
	var jsoncMode os.FileMode
	var jsoncExists bool
	jsoncPath := ""
	if paths.JSONC != "" {
		jsoncPath = filepath.Clean(paths.JSONC)
		jsoncBody, jsoncMode, jsoncExists, err = kernel.files.ReadNoFollow(jsoncPath)
		if err != nil {
			return resolvedFile{}, err
		}
	}
	if jsonExists && jsoncExists {
		return resolvedFile{}, ErrAmbiguousConfig
	}
	if jsoncExists {
		return resolvedFile{path: jsoncPath, body: jsoncBody, mode: jsoncMode, exists: true, jsonc: true}, nil
	}
	return resolvedFile{path: jsonPath, body: jsonBody, mode: jsonMode, exists: jsonExists}, nil
}

func (kernel Kernel) writeVerified(file resolvedFile, next []byte) (resultErr error) {
	release, err := kernel.acquireWriteLock(file.path)
	if err != nil {
		return err
	}
	defer joinUnlock(&resultErr, release)

	mode := file.mode
	if !file.exists {
		mode = 0o600
	}
	restore := func() error {
		if file.exists {
			return kernel.files.WriteAtomic(file.path, file.body, mode)
		}
		return kernel.files.RemoveNoFollow(file.path)
	}
	current, _, exists, err := kernel.files.ReadNoFollow(file.path)
	if err != nil {
		return fmt.Errorf("re-read native config before write: %w", err)
	}
	if exists != file.exists || !bytes.Equal(current, file.body) {
		return ErrConcurrentChange
	}
	if err := kernel.files.WriteAtomic(file.path, next, mode); err != nil {
		return kernel.rollbackIfStillOurs(file, next, restore, fmt.Errorf("write native config: %w", err))
	}
	written, _, exists, err := kernel.files.ReadNoFollow(file.path)
	if err != nil || !exists || !bytes.Equal(written, next) {
		verifyErr := err
		if verifyErr == nil {
			verifyErr = fmt.Errorf("written bytes differ from requested bytes")
		}
		return kernel.rollbackIfStillOurs(file, next, restore, fmt.Errorf("verify native config: %w", verifyErr))
	}
	return nil
}

func (kernel Kernel) rollbackIfStillOurs(file resolvedFile, writtenByUs []byte, restore func() error, primary error) error {
	current, _, exists, readErr := kernel.files.ReadNoFollow(file.path)
	if readErr != nil {
		return errors.Join(primary, fmt.Errorf("rollback skipped because current bytes could not be verified: %w", readErr))
	}
	if exists == file.exists && bytes.Equal(current, file.body) {
		return primary
	}
	if !exists || !bytes.Equal(current, writtenByUs) {
		return errors.Join(primary, fmt.Errorf("rollback skipped because native config is no longer our exact write: %w", ErrConcurrentChange))
	}
	if rollbackErr := restore(); rollbackErr != nil {
		return errors.Join(primary, fmt.Errorf("rollback native config: %w", rollbackErr))
	}
	return fmt.Errorf("native config original bytes restored after failure: %w", primary)
}

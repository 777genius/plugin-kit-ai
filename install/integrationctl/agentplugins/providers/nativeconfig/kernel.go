package nativeconfig

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

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
	if kernel.files == nil {
		return Receipt{}, fmt.Errorf("native config file IO is required")
	}
	if err := validateRequest(req); err != nil {
		return Receipt{}, err
	}
	file, err := kernel.resolve(req.Paths)
	if err != nil {
		return Receipt{}, err
	}
	doc, err := newDocument(file.jsonc)
	if file.exists {
		doc, err = parseDocument(file.body, file.jsonc)
	}
	if err != nil {
		return Receipt{}, err
	}
	key := string(req.Codec)
	if req.Codec == CodecOpenCode {
		key = "mcp"
	}
	entries, err := collection(doc, key, req.Action == ActionAdd)
	if err != nil {
		return Receipt{}, err
	}
	var current *hujson.ObjectMember
	if entries != nil {
		current, _ = objectMember(entries, req.Name)
	}

	switch req.Action {
	case ActionAdd:
		if current != nil {
			return Receipt{}, fmt.Errorf("%w: %s", ErrCollision, req.Name)
		}
		projected, err := projectServer(req.Codec, req.Server, req.Placeholders)
		if err != nil {
			return Receipt{}, err
		}
		value, err := jsonValue(projected)
		if err != nil {
			return Receipt{}, err
		}
		setEntry(entries, req.Name, value)
	case ActionUpdate:
		if current == nil || verifyReceipt(req.Owned, file.path, req.Codec, req.Name, current) != nil {
			return Receipt{}, fmt.Errorf("%w: %s", ErrNotOwned, req.Name)
		}
		projected, err := projectServer(req.Codec, req.Server, req.Placeholders)
		if err != nil {
			return Receipt{}, err
		}
		value, err := jsonValue(projected)
		if err != nil {
			return Receipt{}, err
		}
		setEntry(entries, req.Name, value)
	case ActionRemove:
		if current == nil || verifyReceipt(req.Owned, file.path, req.Codec, req.Name, current) != nil {
			return Receipt{}, fmt.Errorf("%w: %s", ErrNotOwned, req.Name)
		}
		removeEntry(entries, req.Name)
	}

	next, err := doc.render()
	if err != nil {
		return Receipt{}, err
	}
	if err := kernel.writeVerified(file, next); err != nil {
		return Receipt{}, err
	}
	if req.Action == ActionRemove {
		return Receipt{}, nil
	}
	member, _ := objectMember(entries, req.Name)
	digest, err := entryDigest(req.Codec, req.Name, member)
	if err != nil {
		return Receipt{}, err
	}
	return Receipt{Version: "1", Path: file.path, Codec: req.Codec, Name: req.Name, Digest: digest}, nil
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

func (kernel Kernel) writeVerified(file resolvedFile, next []byte) error {
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
		if rollbackErr := restore(); rollbackErr != nil {
			return errors.Join(fmt.Errorf("write native config: %w", err), fmt.Errorf("rollback native config: %w", rollbackErr))
		}
		return fmt.Errorf("write native config (original bytes restored): %w", err)
	}
	written, _, exists, err := kernel.files.ReadNoFollow(file.path)
	if err != nil || !exists || !bytes.Equal(written, next) {
		verifyErr := err
		if verifyErr == nil {
			verifyErr = fmt.Errorf("written bytes differ from requested bytes")
		}
		if rollbackErr := restore(); rollbackErr != nil {
			return errors.Join(fmt.Errorf("verify native config: %w", verifyErr), fmt.Errorf("rollback native config: %w", rollbackErr))
		}
		return fmt.Errorf("verify native config (original bytes restored): %w", verifyErr)
	}
	return nil
}

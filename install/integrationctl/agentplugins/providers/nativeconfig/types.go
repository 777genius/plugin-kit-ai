// Package nativeconfig applies ownership-aware MCP server patches to native
// JSON and JSONC client configuration files.
package nativeconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Codec string

const (
	CodecMCPServers Codec = "mcpServers"
	CodecOpenCode   Codec = "opencode-mcp"
)

type Action string

const (
	ActionAdd    Action = "add"
	ActionUpdate Action = "update"
	ActionRemove Action = "remove"
)

var (
	ErrAmbiguousConfig  = errors.New("both JSON and JSONC native config files exist")
	ErrCollision        = errors.New("native MCP entry already exists")
	ErrNotOwned         = errors.New("native MCP entry is not exactly owned")
	ErrMalformed        = errors.New("native config is malformed")
	ErrConcurrentChange = errors.New("native config changed during patch")
)

// Server is the neutral MCP shape accepted by both codecs. Type must be
// "stdio" or "remote". Placeholders are resolved recursively in all strings.
type Server struct {
	Type    string            `json:"type"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// Paths names the mutually exclusive client config variants. If neither
// exists, JSON is created. The paths must be explicit and must not use HOME.
type Paths struct {
	JSON  string
	JSONC string
}

type Placeholders struct {
	PackageRoot string
	DataRoot    string
}

// Receipt identifies one exact projected entry. It is intentionally safe to
// persist: it contains no original file bytes or unrelated configuration.
type Receipt struct {
	Version string `json:"version"`
	Path    string `json:"path"`
	Codec   Codec  `json:"codec"`
	Name    string `json:"name"`
	Digest  string `json:"digest"`
}

type Request struct {
	Paths        Paths
	Codec        Codec
	Action       Action
	Name         string
	Server       Server
	Placeholders Placeholders
	Owned        *Receipt
}

type FileIO interface {
	ReadNoFollow(path string) (body []byte, mode os.FileMode, exists bool, err error)
	WriteAtomic(path string, body []byte, mode os.FileMode) error
	RemoveNoFollow(path string) error
}

type Kernel struct{ files FileIO }

func New() Kernel { return Kernel{files: osFiles{}} }

func NewWithFileIO(files FileIO) Kernel { return Kernel{files: files} }

func validateRequest(req Request) error {
	if strings.TrimSpace(req.Paths.JSON) == "" {
		return fmt.Errorf("JSON native config path is required")
	}
	if !filepath.IsAbs(req.Paths.JSON) || (req.Paths.JSONC != "" && !filepath.IsAbs(req.Paths.JSONC)) {
		return fmt.Errorf("native config paths must be absolute")
	}
	if req.Paths.JSONC != "" && filepath.Clean(req.Paths.JSON) == filepath.Clean(req.Paths.JSONC) {
		return fmt.Errorf("JSON and JSONC native config paths must differ")
	}
	if strings.TrimSpace(req.Name) == "" {
		return fmt.Errorf("MCP entry name is required")
	}
	if req.Codec != CodecMCPServers && req.Codec != CodecOpenCode {
		return fmt.Errorf("unsupported native config codec %q", req.Codec)
	}
	if req.Action != ActionAdd && req.Action != ActionUpdate && req.Action != ActionRemove {
		return fmt.Errorf("unsupported native config action %q", req.Action)
	}
	if req.Action != ActionAdd && req.Owned == nil {
		return fmt.Errorf("%s requires an ownership receipt: %w", req.Action, ErrNotOwned)
	}
	return nil
}

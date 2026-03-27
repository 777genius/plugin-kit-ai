package validate

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type projectManifest struct {
	SchemaVersion int
	Platform      string
	Runtime       string
	ExecutionMode string
	Entrypoint    string
}

func loadManifest(root string) (projectManifest, error) {
	full := filepath.Join(root, ".plugin-kit-ai", "project.toml")
	body, err := os.ReadFile(full)
	if err != nil {
		return projectManifest{}, err
	}
	return parseManifest(body)
}

func parseManifest(body []byte) (projectManifest, error) {
	var out projectManifest
	seen := map[string]bool{}
	lines := strings.Split(string(body), "\n")
	for idx, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return projectManifest{}, fmt.Errorf("line %d: expected key = value", idx+1)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		switch key {
		case "schema_version":
			n, err := strconv.Atoi(value)
			if err != nil {
				return projectManifest{}, fmt.Errorf("line %d: invalid schema_version", idx+1)
			}
			out.SchemaVersion = n
		case "platform":
			s, err := parseQuotedTOMLString(value)
			if err != nil {
				return projectManifest{}, fmt.Errorf("line %d: invalid platform: %w", idx+1, err)
			}
			out.Platform = s
		case "runtime":
			s, err := parseQuotedTOMLString(value)
			if err != nil {
				return projectManifest{}, fmt.Errorf("line %d: invalid runtime: %w", idx+1, err)
			}
			out.Runtime = s
		case "execution_mode":
			s, err := parseQuotedTOMLString(value)
			if err != nil {
				return projectManifest{}, fmt.Errorf("line %d: invalid execution_mode: %w", idx+1, err)
			}
			out.ExecutionMode = s
		case "entrypoint":
			s, err := parseQuotedTOMLString(value)
			if err != nil {
				return projectManifest{}, fmt.Errorf("line %d: invalid entrypoint: %w", idx+1, err)
			}
			out.Entrypoint = s
		}
		seen[key] = true
	}

	for _, key := range []string{"schema_version", "platform", "runtime", "execution_mode", "entrypoint"} {
		if !seen[key] {
			return projectManifest{}, fmt.Errorf("missing %s", key)
		}
	}
	return out, nil
}

func parseQuotedTOMLString(v string) (string, error) {
	if len(v) < 2 || !strings.HasPrefix(v, `"`) || !strings.HasSuffix(v, `"`) {
		return "", fmt.Errorf("expected quoted string")
	}
	s, err := strconv.Unquote(v)
	if err != nil {
		return "", err
	}
	return s, nil
}

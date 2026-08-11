package agentpluginscli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/spf13/cobra"
)

type batchTargetResult struct {
	Target string `json:"target"`
	Output any    `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

type batchCommandResult struct {
	Batch     bool                `json:"batch"`
	Succeeded int                 `json:"succeeded"`
	Failed    int                 `json:"failed"`
	Targets   []batchTargetResult `json:"targets"`
}

func parseTargetOption(value string) ([]domain.ClientID, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}

	parts := strings.Split(trimmed, ",")
	targets := make([]domain.ClientID, 0, len(parts))
	seen := make(map[domain.ClientID]struct{}, len(parts))
	for _, part := range parts {
		raw := strings.TrimSpace(part)
		if raw == "" {
			return nil, fmt.Errorf("--target contains an empty client; use a comma-separated list such as codex,cursor")
		}
		if strings.EqualFold(raw, "all") {
			return nil, fmt.Errorf("--target all is not supported; choose clients explicitly")
		}
		if strings.EqualFold(raw, "openai") {
			return nil, fmt.Errorf("target %q is ambiguous; use codex, chatgpt, or both", raw)
		}

		target := normalizeTarget(raw)
		if target == "legacy-all" {
			if len(parts) != 1 {
				return nil, fmt.Errorf("--target legacy-all cannot be combined with Agent Plugins clients")
			}
			return []domain.ClientID{target}, nil
		}
		if !supportedTarget(target) {
			return nil, fmt.Errorf("unsupported target %q; choose codex, chatgpt, cursor, copilot, vscode, or kiro", raw)
		}
		if _, duplicate := seen[target]; duplicate {
			continue
		}
		seen[target] = struct{}{}
		targets = append(targets, target)
	}
	return targets, nil
}

func supportedTarget(target domain.ClientID) bool {
	switch target {
	case domain.ClientCodex, domain.ClientChatGPT, domain.ClientCursor, domain.ClientCopilot, domain.ClientVSCode, domain.ClientKiro:
		return true
	default:
		return false
	}
}

func runForTargets(cmd *cobra.Command, opts *options, command string, run func() error) error {
	targets, err := parseTargetOption(opts.target)
	if err != nil {
		return err
	}
	if len(targets) <= 1 {
		if len(targets) == 1 {
			original := opts.target
			opts.target = string(targets[0])
			defer func() { opts.target = original }()
		}
		return run()
	}

	originalTarget := opts.target
	originalOutput := cmd.OutOrStdout()
	defer func() {
		opts.target = originalTarget
		cmd.SetOut(originalOutput)
	}()

	result := batchCommandResult{Batch: true, Targets: make([]batchTargetResult, 0, len(targets))}
	var failures []string
	if opts.format == "human" {
		_, _ = fmt.Fprintf(originalOutput, "Running %s for %d targets: %s\n", command, len(targets), joinTargets(targets))
	}

	for index, target := range targets {
		opts.target = string(target)
		entry := batchTargetResult{Target: string(target)}
		var captured bytes.Buffer
		if opts.format == "json" {
			cmd.SetOut(&captured)
		} else {
			_, _ = fmt.Fprintf(originalOutput, "\n[%d/%d] %s\n", index+1, len(targets), target)
		}

		runErr := run()
		if opts.format == "json" {
			cmd.SetOut(originalOutput)
			if payload := bytes.TrimSpace(captured.Bytes()); len(payload) > 0 {
				if jsonErr := json.Unmarshal(payload, &entry.Output); jsonErr != nil {
					runErr = fmt.Errorf("invalid %s JSON for target %s: %w", command, target, jsonErr)
				}
			}
		}
		if runErr != nil {
			entry.Error = "command failed for this target; see the process error for details"
			result.Failed++
			failures = append(failures, fmt.Sprintf("%s: %v", target, runErr))
			if opts.format == "human" {
				_, _ = fmt.Fprintf(originalOutput, "Target %s failed: %v\n", target, runErr)
			}
		} else {
			result.Succeeded++
		}
		result.Targets = append(result.Targets, entry)
	}

	if opts.format == "json" {
		if err := writeJSONOutput(originalOutput, command, result); err != nil {
			return err
		}
	} else {
		_, _ = fmt.Fprintf(originalOutput, "\nCompleted: %d succeeded, %d failed.\n", result.Succeeded, result.Failed)
	}
	if len(failures) > 0 {
		return fmt.Errorf("%s failed for %d of %d targets: %s", command, len(failures), len(targets), strings.Join(failures, "; "))
	}
	return nil
}

func joinTargets(targets []domain.ClientID) string {
	values := make([]string, len(targets))
	for index, target := range targets {
		values[index] = string(target)
	}
	return strings.Join(values, ", ")
}

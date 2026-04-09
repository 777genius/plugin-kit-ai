package source

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

type stubRunner struct {
	run func(context.Context, ports.Command) (ports.CommandResult, error)
}

func (s stubRunner) Run(ctx context.Context, cmd ports.Command) (ports.CommandResult, error) {
	return s.run(ctx, cmd)
}

func TestResolveGitURLUsesProcessRunnerAndHashesMaterializedTree(t *testing.T) {
	t.Parallel()
	resolver := Resolver{
		Runner: stubRunner{run: func(_ context.Context, cmd ports.Command) (ports.CommandResult, error) {
			if len(cmd.Argv) >= 5 && cmd.Argv[0] == "git" && cmd.Argv[1] == "clone" {
				dst := cmd.Argv[len(cmd.Argv)-1]
				if err := os.MkdirAll(filepath.Join(dst, "src"), 0o755); err != nil {
					t.Fatalf("mkdir clone dst: %v", err)
				}
				if err := os.WriteFile(filepath.Join(dst, "src", "plugin.yaml"), []byte("api_version: v1\nname: demo\nversion: 0.1.0\ndescription: test\ntargets:\n  - cursor\n"), 0o644); err != nil {
					t.Fatalf("write clone plugin.yaml: %v", err)
				}
				return ports.CommandResult{ExitCode: 0}, nil
			}
			if len(cmd.Argv) >= 5 && cmd.Argv[0] == "git" && cmd.Argv[3] == "rev-parse" {
				return ports.CommandResult{ExitCode: 0, Stdout: []byte("abc123\n")}, nil
			}
			t.Fatalf("unexpected command: %+v", cmd.Argv)
			return ports.CommandResult{}, nil
		}},
	}

	resolved, err := resolver.Resolve(context.Background(), domain.IntegrationRef{Raw: "https://example.com/acme/demo.git"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.Kind != "git_url" {
		t.Fatalf("kind = %s, want git_url", resolved.Kind)
	}
	if resolved.CleanupPath == "" {
		t.Fatal("expected cleanup path")
	}
	if !strings.HasSuffix(resolved.Resolved.Value, "@abc123") {
		t.Fatalf("resolved ref = %s", resolved.Resolved.Value)
	}
	wantDigest, err := hashLocalTree(resolved.LocalPath)
	if err != nil {
		t.Fatalf("hash materialized tree: %v", err)
	}
	if resolved.SourceDigest != wantDigest {
		t.Fatalf("source digest = %s, want %s", resolved.SourceDigest, wantDigest)
	}
}

func TestResolveGitHubSubdirUsesSubtreeDigestAndCleanupRoot(t *testing.T) {
	t.Parallel()
	resolver := Resolver{
		Runner: stubRunner{run: func(_ context.Context, cmd ports.Command) (ports.CommandResult, error) {
			if len(cmd.Argv) >= 5 && cmd.Argv[0] == "git" && cmd.Argv[1] == "clone" {
				dst := cmd.Argv[len(cmd.Argv)-1]
				if err := os.MkdirAll(filepath.Join(dst, "plugins", "demo", "src"), 0o755); err != nil {
					t.Fatalf("mkdir clone dst: %v", err)
				}
				if err := os.WriteFile(filepath.Join(dst, "README.md"), []byte("root readme\n"), 0o644); err != nil {
					t.Fatalf("write clone readme: %v", err)
				}
				if err := os.WriteFile(filepath.Join(dst, "plugins", "demo", "src", "plugin.yaml"), []byte("api_version: v1\nname: demo\nversion: 0.1.0\ndescription: test\ntargets:\n  - opencode\n"), 0o644); err != nil {
					t.Fatalf("write subtree plugin.yaml: %v", err)
				}
				return ports.CommandResult{ExitCode: 0}, nil
			}
			if len(cmd.Argv) >= 5 && cmd.Argv[0] == "git" && cmd.Argv[3] == "rev-parse" {
				return ports.CommandResult{ExitCode: 0, Stdout: []byte("def456\n")}, nil
			}
			t.Fatalf("unexpected command: %+v", cmd.Argv)
			return ports.CommandResult{}, nil
		}},
	}

	resolved, err := resolver.Resolve(context.Background(), domain.IntegrationRef{Raw: "github:acme/demo//plugins/demo"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.Kind != "github_repo_path" {
		t.Fatalf("kind = %s, want github_repo_path", resolved.Kind)
	}
	if filepath.Base(resolved.LocalPath) != "demo" {
		t.Fatalf("local path = %s, want subdir root", resolved.LocalPath)
	}
	if filepath.Clean(resolved.CleanupPath) == filepath.Clean(resolved.LocalPath) {
		t.Fatalf("cleanup path should point at clone root, got %s", resolved.CleanupPath)
	}
	wantDigest, err := hashLocalTree(resolved.LocalPath)
	if err != nil {
		t.Fatalf("hash subtree: %v", err)
	}
	if resolved.SourceDigest != wantDigest {
		t.Fatalf("source digest = %s, want %s", resolved.SourceDigest, wantDigest)
	}
}

func TestResolveFirstPartyAliasMapsToCanonicalGitHubSource(t *testing.T) {
	t.Parallel()
	resolver := Resolver{
		Runner: stubRunner{run: func(_ context.Context, cmd ports.Command) (ports.CommandResult, error) {
			if len(cmd.Argv) >= 5 && cmd.Argv[0] == "git" && cmd.Argv[1] == "clone" {
				if got, want := cmd.Argv[4], "https://github.com/777genius/universal-plugins-for-ai-agents.git"; got != want {
					t.Fatalf("clone url = %q, want %q", got, want)
				}
				dst := cmd.Argv[len(cmd.Argv)-1]
				if err := os.MkdirAll(filepath.Join(dst, "plugins", "notion", "src"), 0o755); err != nil {
					t.Fatalf("mkdir clone dst: %v", err)
				}
				if err := os.WriteFile(filepath.Join(dst, "plugins", "notion", "src", "plugin.yaml"), []byte("api_version: v1\nname: notion\nversion: 0.1.0\ndescription: test\ntargets:\n  - claude\n"), 0o644); err != nil {
					t.Fatalf("write subtree plugin.yaml: %v", err)
				}
				return ports.CommandResult{ExitCode: 0}, nil
			}
			if len(cmd.Argv) >= 5 && cmd.Argv[0] == "git" && cmd.Argv[3] == "rev-parse" {
				return ports.CommandResult{ExitCode: 0, Stdout: []byte("alias123\n")}, nil
			}
			t.Fatalf("unexpected command: %+v", cmd.Argv)
			return ports.CommandResult{}, nil
		}},
	}

	resolved, err := resolver.Resolve(context.Background(), domain.IntegrationRef{Raw: "notion"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.Kind != "github_repo_path" {
		t.Fatalf("kind = %s, want github_repo_path", resolved.Kind)
	}
	if got, want := filepath.Base(resolved.LocalPath), "notion"; got != want {
		t.Fatalf("local path = %q, want %q", got, want)
	}
}

func TestResolveGitHubRefFetchesPinnedRevision(t *testing.T) {
	t.Parallel()
	var sawFetch bool
	resolver := Resolver{
		Runner: stubRunner{run: func(_ context.Context, cmd ports.Command) (ports.CommandResult, error) {
			switch {
			case len(cmd.Argv) >= 5 && cmd.Argv[0] == "git" && cmd.Argv[1] == "clone":
				dst := cmd.Argv[len(cmd.Argv)-1]
				if err := os.MkdirAll(filepath.Join(dst, "plugin"), 0o755); err != nil {
					t.Fatalf("mkdir clone dst: %v", err)
				}
				if err := os.WriteFile(filepath.Join(dst, "plugin", "README.md"), []byte("demo\n"), 0o644); err != nil {
					t.Fatalf("write clone readme: %v", err)
				}
				return ports.CommandResult{ExitCode: 0}, nil
			case len(cmd.Argv) >= 7 && cmd.Argv[0] == "git" && cmd.Argv[3] == "fetch":
				sawFetch = true
				if got := cmd.Argv[len(cmd.Argv)-1]; got != "v1.2.3" {
					t.Fatalf("fetch ref = %q, want v1.2.3", got)
				}
				return ports.CommandResult{ExitCode: 0}, nil
			case len(cmd.Argv) >= 5 && cmd.Argv[0] == "git" && cmd.Argv[3] == "checkout":
				if got := cmd.Argv[len(cmd.Argv)-1]; got != "FETCH_HEAD" {
					t.Fatalf("checkout target = %q, want FETCH_HEAD", got)
				}
				return ports.CommandResult{ExitCode: 0}, nil
			case len(cmd.Argv) >= 5 && cmd.Argv[0] == "git" && cmd.Argv[3] == "rev-parse":
				return ports.CommandResult{ExitCode: 0, Stdout: []byte("fedcba\n")}, nil
			default:
				t.Fatalf("unexpected command: %+v", cmd.Argv)
				return ports.CommandResult{}, nil
			}
		}},
	}

	resolved, err := resolver.Resolve(context.Background(), domain.IntegrationRef{Raw: "github:acme/demo@v1.2.3//plugin"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !sawFetch {
		t.Fatal("expected pinned ref fetch")
	}
	if resolved.Resolved.Value != "https://github.com/acme/demo@fedcba" {
		t.Fatalf("resolved ref = %s", resolved.Resolved.Value)
	}
}

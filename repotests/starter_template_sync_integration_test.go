package pluginkitairepo_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStarterTemplateSyncContractFilesStayAligned(t *testing.T) {
	root := RepoRoot(t)

	mapping := readRepoFile(t, root, "examples", "starters", "template-repos.txt")
	script := readRepoFile(t, root, "scripts", "update-starter-template.sh")
	workflow := readRepoFile(t, root, ".github", "workflows", "starter-templates.yml")
	rootReadme := readRepoFile(t, root, "README.md")
	cliReadme := readRepoFile(t, root, "cli", "plugin-kit-ai", "README.md")
	startersReadme := readRepoFile(t, root, "examples", "starters", "README.md")

	expected := map[string]string{
		"codex-go-starter":               "plugin-kit-ai-starter-codex-go",
		"codex-python-starter":           "plugin-kit-ai-starter-codex-python",
		"codex-node-typescript-starter":  "plugin-kit-ai-starter-codex-node-typescript",
		"claude-go-starter":              "plugin-kit-ai-starter-claude-go",
		"claude-python-starter":          "plugin-kit-ai-starter-claude-python",
		"claude-node-typescript-starter": "plugin-kit-ai-starter-claude-node-typescript",
	}

	for starter, repo := range expected {
		mustContain(t, mapping, starter+" "+repo)
		mustContain(t, workflow, "- "+starter)
		mustContain(t, startersReadme, "https://github.com/777genius/"+repo)
	}
	mustContain(t, script, "STARTER_TEMPLATE_SYNC_TOKEN")
	mustContain(t, script, "template-repos.txt")
	mustContain(t, script, "find \"$tmp/repo\" -mindepth 1 -maxdepth 1 ! -name '.git' -exec rm -rf {} +")
	mustContain(t, workflow, "STARTER_TEMPLATE_SYNC_TOKEN")
	mustContain(t, workflow, "run: ./scripts/update-starter-template.sh")
	mustContain(t, workflow, "default: \"all\"")
	mustContain(t, rootReadme, "Official starter templates:")
	mustContain(t, cliReadme, "Official starter templates:")
	mustContain(t, startersReadme, "These in-repo starter folders are the canonical source of truth.")
	mustContain(t, startersReadme, "Use this template")
}

func TestStarterTemplateSyncScriptSupportsLocalMirror(t *testing.T) {
	root := RepoRoot(t)
	workDir := t.TempDir()
	remoteBase := filepath.Join(workDir, "remotes")
	if err := os.MkdirAll(remoteBase, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, repo := range []string{
		"plugin-kit-ai-starter-codex-go",
		"plugin-kit-ai-starter-codex-python",
		"plugin-kit-ai-starter-codex-node-typescript",
		"plugin-kit-ai-starter-claude-go",
		"plugin-kit-ai-starter-claude-python",
		"plugin-kit-ai-starter-claude-node-typescript",
	} {
		remote := filepath.Join(remoteBase, repo+".git")
		if out, err := exec.Command("git", "init", "--bare", remote).CombinedOutput(); err != nil {
			t.Fatalf("git init bare %s: %v\n%s", repo, err, out)
		}

		seedDir := filepath.Join(workDir, repo+"-seed")
		if out, err := exec.Command("git", "clone", remote, seedDir).CombinedOutput(); err != nil {
			t.Fatalf("git clone seed %s: %v\n%s", repo, err, out)
		}
		writeRuntimeFile(t, seedDir, "README.md", "seed\n")
		if out, err := exec.Command("git", "-C", seedDir, "config", "user.name", "test-bot").CombinedOutput(); err != nil {
			t.Fatalf("git config user.name: %v\n%s", err, out)
		}
		if out, err := exec.Command("git", "-C", seedDir, "config", "user.email", "test@example.com").CombinedOutput(); err != nil {
			t.Fatalf("git config user.email: %v\n%s", err, out)
		}
		if out, err := exec.Command("git", "-C", seedDir, "add", "-A").CombinedOutput(); err != nil {
			t.Fatalf("git add seed: %v\n%s", err, out)
		}
		if out, err := exec.Command("git", "-C", seedDir, "commit", "-m", "seed").CombinedOutput(); err != nil {
			t.Fatalf("git commit seed: %v\n%s", err, out)
		}
		if out, err := exec.Command("git", "-C", seedDir, "push", "origin", "HEAD:main").CombinedOutput(); err != nil {
			t.Fatalf("git push seed: %v\n%s", err, out)
		}
	}

	cmd := exec.Command("bash", "./scripts/update-starter-template.sh")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"STARTER=all",
		"STARTER_TEMPLATE_REMOTE_BASE="+remoteBase,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("update-starter-template.sh: %v\n%s", err, out)
	}

	checks := map[string][]string{
		"plugin-kit-ai-starter-codex-go":              {"plugin.yaml", "go.mod", "cmd/codex-go-starter/main.go", "targets/codex-runtime/package.yaml"},
		"plugin-kit-ai-starter-codex-python":          {"plugin.yaml", "requirements.txt", "bin/codex-python-starter", "bin/codex-python-starter.cmd", "targets/codex-runtime/package.yaml", ".github/workflows/bundle-release.yml"},
		"plugin-kit-ai-starter-codex-node-typescript": {"plugin.yaml", "package.json", "bin/codex-node-typescript-starter", "bin/codex-node-typescript-starter.cmd", "tsconfig.json", "targets/codex-runtime/package.yaml"},
		"plugin-kit-ai-starter-claude-go":             {"plugin.yaml", "go.mod", ".claude-plugin/plugin.json", "targets/claude/hooks/hooks.json"},
		"plugin-kit-ai-starter-claude-python":         {"plugin.yaml", "requirements.txt", "bin/claude-python-starter", "bin/claude-python-starter.cmd", ".claude-plugin/plugin.json", "targets/claude/hooks/hooks.json"},
		"plugin-kit-ai-starter-claude-node-typescript": {
			"plugin.yaml", "package.json", "bin/claude-node-typescript-starter", "bin/claude-node-typescript-starter.cmd", ".claude-plugin/plugin.json", "targets/claude/hooks/hooks.json",
		},
	}

	for repo, required := range checks {
		cloneDir := filepath.Join(workDir, repo+"-check")
		remote := filepath.Join(remoteBase, repo+".git")
		if out, err := exec.Command("git", "clone", "--branch", "main", remote, cloneDir).CombinedOutput(); err != nil {
			t.Fatalf("git clone check %s: %v\n%s", repo, err, out)
		}
		for _, rel := range required {
			if !fileExists(filepath.Join(cloneDir, rel)) {
				t.Fatalf("%s missing %s after sync", repo, rel)
			}
		}
		if fileExists(filepath.Join(cloneDir, ".git", "README.md")) {
			t.Fatalf("%s unexpectedly copied nested .git payload", repo)
		}
	}
}

func TestStarterTemplateRepoLinksResolveToCurrentOwnerNaming(t *testing.T) {
	root := RepoRoot(t)
	landing := readRepoFile(t, root, "examples", "starters", "README.md")
	lines := strings.Split(landing, "\n")
	var found int
	for _, line := range lines {
		if strings.Contains(line, "https://github.com/777genius/plugin-kit-ai-starter-") {
			found++
		}
	}
	if found != 6 {
		t.Fatalf("expected 6 external starter template links, found %d\n%s", found, landing)
	}
}

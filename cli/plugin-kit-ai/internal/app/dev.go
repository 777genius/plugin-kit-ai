package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/runtimecheck"
	"github.com/777genius/plugin-kit-ai/cli/internal/validate"
)

var devCommandContext = exec.CommandContext

type PluginDevOptions struct {
	Root      string
	Platform  string
	Event     string
	Fixture   string
	GoldenDir string
	All       bool
	Once      bool
	Interval  time.Duration
}

type PluginDevUpdate struct {
	Cycle  int
	Passed bool
	Lines  []string
}

type PluginDevSummary struct {
	Cycles     int
	LastPassed bool
}

type devSnapshot map[string]devFileState

type devFileState struct {
	Size    int64
	Mode    os.FileMode
	ModTime int64
}

func (svc PluginService) Dev(ctx context.Context, opts PluginDevOptions, emit func(PluginDevUpdate)) (PluginDevSummary, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = 750 * time.Millisecond
	}
	if emit == nil {
		emit = func(PluginDevUpdate) {}
	}

	selectedPlatform, err := resolveDevPlatform(root, opts.Platform)
	if err != nil {
		return PluginDevSummary{}, err
	}

	cycle := 0
	runCycle := func(trigger string, changed []string) (bool, error) {
		cycle++
		update := svc.runDevCycle(ctx, root, selectedPlatform, opts, cycle, trigger, changed)
		emit(update)
		return update.Passed, nil
	}

	lastPassed, err := runCycle("initial", nil)
	if err != nil {
		return PluginDevSummary{}, err
	}
	if opts.Once {
		return PluginDevSummary{Cycles: cycle, LastPassed: lastPassed}, nil
	}

	snapshot, err := takeDevSnapshot(root)
	if err != nil {
		return PluginDevSummary{}, err
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return PluginDevSummary{Cycles: cycle, LastPassed: lastPassed}, nil
		case <-ticker.C:
			next, err := takeDevSnapshot(root)
			if err != nil {
				update := PluginDevUpdate{
					Cycle:  cycle + 1,
					Passed: false,
					Lines:  []string{fmt.Sprintf("Cycle %d [watch]: snapshot failed: %v", cycle+1, err)},
				}
				cycle++
				lastPassed = false
				emit(update)
				continue
			}
			changed := devSnapshotChanges(snapshot, next)
			if len(changed) == 0 {
				continue
			}
			lastPassed, _ = runCycle("watch", changed)
			snapshot, err = takeDevSnapshot(root)
			if err != nil {
				return PluginDevSummary{Cycles: cycle, LastPassed: lastPassed}, err
			}
		}
	}
}

func resolveDevPlatform(root, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		if !isRuntimeTestPlatform(requested) {
			return "", runtimeDevUnsupportedPlatformError(nil, requested)
		}
		return strings.ToLower(requested), nil
	}
	graph, _, err := pluginmanifest.Discover(root)
	if err != nil {
		return "", err
	}
	enabledTargets := graph.Manifest.EnabledTargets()
	platform, err := resolveRuntimeTestPlatform(enabledTargets, "")
	if err != nil {
		return "", err
	}
	return platform, nil
}

func (svc PluginService) runDevCycle(ctx context.Context, root, platform string, opts PluginDevOptions, cycle int, trigger string, changed []string) PluginDevUpdate {
	lines := []string{devCycleHeader(cycle, trigger, changed)}
	update := PluginDevUpdate{Cycle: cycle, Passed: false}

	graph, _, err := pluginmanifest.Discover(root)
	if err != nil {
		lines = append(lines, "Discover: "+err.Error())
		update.Lines = lines
		return update
	}
	if _, err := graph.Manifest.SelectedTargets(platform); err != nil {
		lines = append(lines, err.Error())
		update.Lines = lines
		return update
	}
	if graph.Launcher == nil {
		lines = append(lines, "launcher invalid: missing launcher.yaml")
		update.Lines = lines
		return update
	}

	rendered, err := svc.Render(PluginRenderOptions{Root: root, Target: platform})
	if err != nil {
		lines = append(lines, "Render: "+err.Error())
		update.Lines = lines
		return update
	}
	lines = append(lines, fmt.Sprintf("Render: wrote %d artifact(s)", len(rendered)))

	project, err := runtimecheck.Inspect(runtimecheck.Inputs{
		Root:     root,
		Targets:  []string{platform},
		Launcher: graph.Launcher,
	})
	if err != nil {
		lines = append(lines, "Runtime inspect: "+err.Error())
		update.Lines = lines
		return update
	}
	if buildLines, err := devAutoBuild(ctx, root, graph, project); err != nil {
		lines = append(lines, buildLines...)
		lines = append(lines, "Build: "+err.Error())
		update.Lines = lines
		return update
	} else if len(buildLines) > 0 {
		lines = append(lines, buildLines...)
	}

	report, err := validate.Validate(root, platform)
	if err != nil {
		if re, ok := err.(*validate.ReportError); ok {
			report = re.Report
		} else {
			lines = append(lines, "Validate: "+err.Error())
			update.Lines = lines
			return update
		}
	}
	if len(report.Failures) > 0 {
		lines = append(lines, fmt.Sprintf("Validate: %d failure(s)", len(report.Failures)))
		for _, failure := range report.Failures {
			lines = append(lines, "  - "+failure.Message)
		}
		update.Lines = lines
		return update
	}
	if len(report.Warnings) > 0 {
		lines = append(lines, fmt.Sprintf("Validate: %d warning(s) (strict)", len(report.Warnings)))
		for _, warning := range report.Warnings {
			lines = append(lines, "  - "+warning.Message)
		}
		update.Lines = lines
		return update
	}
	lines = append(lines, "Validate: ok")

	project, err = runtimecheck.Inspect(runtimecheck.Inputs{
		Root:     root,
		Targets:  []string{platform},
		Launcher: graph.Launcher,
	})
	if err != nil {
		lines = append(lines, "Runtime inspect: "+err.Error())
		update.Lines = lines
		return update
	}
	diagnosis := runtimecheck.Diagnose(project)
	lines = append(lines, project.ProjectLine())
	lines = append(lines, fmt.Sprintf("Runtime: %s (%s)", diagnosis.Status, diagnosis.Reason))
	if diagnosis.Status != runtimecheck.StatusReady {
		update.Lines = lines
		return update
	}

	supported := stableRuntimeSupport(platform)
	selected, err := selectRuntimeTestCases(supported, opts.Event, opts.All)
	if err != nil {
		lines = append(lines, err.Error())
		update.Lines = lines
		return update
	}
	if opts.All && strings.TrimSpace(opts.Fixture) != "" {
		lines = append(lines, "--fixture cannot be used with --all")
		update.Lines = lines
		return update
	}

	passed := true
	for _, item := range selected {
		tc := runRuntimeTestCase(ctx, root, project, PluginTestOptions{
			Root:      root,
			Platform:  platform,
			Event:     opts.Event,
			Fixture:   opts.Fixture,
			GoldenDir: opts.GoldenDir,
			All:       opts.All,
		}, item)
		lines = append(lines, formatRuntimeTestCaseLine(tc))
		if strings.TrimSpace(tc.Stdout) != "" {
			lines = append(lines, "  stdout: "+singleLinePreview(tc.Stdout))
		}
		if strings.TrimSpace(tc.Stderr) != "" {
			lines = append(lines, "  stderr: "+singleLinePreview(tc.Stderr))
		}
		if tc.Failure != "" {
			passed = false
			continue
		}
		if !tc.Passed {
			passed = false
		}
	}

	update.Passed = passed
	update.Lines = lines
	return update
}

func devCycleHeader(cycle int, trigger string, changed []string) string {
	label := fmt.Sprintf("Cycle %d [%s]", cycle, trigger)
	if len(changed) == 0 {
		return label
	}
	if len(changed) <= 5 {
		return label + " change(s): " + strings.Join(changed, ", ")
	}
	return label + " change(s): " + strings.Join(changed[:5], ", ") + fmt.Sprintf(" (+%d more)", len(changed)-5)
}

func takeDevSnapshot(root string) (devSnapshot, error) {
	out := make(devSnapshot)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if info.IsDir() {
			if shouldSkipDevDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		out[rel] = devFileState{
			Size:    info.Size(),
			Mode:    info.Mode(),
			ModTime: info.ModTime().UnixNano(),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func shouldSkipDevDir(rel string) bool {
	base := filepath.Base(rel)
	switch base {
	case ".git", "node_modules", ".venv", "__pycache__":
		return true
	default:
		return false
	}
}

func devSnapshotChanges(prev, next devSnapshot) []string {
	set := map[string]struct{}{}
	for path, state := range next {
		if old, ok := prev[path]; !ok || old != state {
			set[path] = struct{}{}
		}
	}
	for path := range prev {
		if _, ok := next[path]; !ok {
			set[path] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for path := range set {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func devAutoBuild(ctx context.Context, root string, graph pluginmanifest.PackageGraph, project runtimecheck.Project) ([]string, error) {
	switch project.Runtime {
	case "go":
		return devBuildGo(ctx, root, graph)
	case "node":
		if strings.TrimSpace(project.Node.BuildScript) != "" {
			return devBuildNode(ctx, root, project.Node)
		}
	}
	return nil, nil
}

func devBuildGo(ctx context.Context, root string, graph pluginmanifest.PackageGraph) ([]string, error) {
	entrypoint := strings.TrimSpace(graph.Launcher.Entrypoint)
	if entrypoint == "" {
		return nil, fmt.Errorf("build requires launcher entrypoint")
	}
	output := filepath.Join(root, strings.TrimPrefix(filepath.Clean(entrypoint), "./"))
	target := filepath.Join(".", "cmd", graph.Manifest.Name)
	if _, err := os.Stat(filepath.Join(root, "cmd", graph.Manifest.Name)); err != nil {
		return nil, fmt.Errorf("build requires %s for automatic Go rebuild", filepath.ToSlash(filepath.Join("cmd", graph.Manifest.Name)))
	}
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(output), ".exe") {
		output += ".exe"
	}
	cmd := devCommandContext(ctx, "go", "build", "-o", output, target)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		return []string{"Build: go build -o " + filepath.ToSlash(output) + " " + filepath.ToSlash(target)}, fmt.Errorf("go build failed: %v\n%s", err, out)
	}
	return []string{"Build: go build -o " + filepath.ToSlash(output) + " " + filepath.ToSlash(target)}, nil
}

func devBuildNode(ctx context.Context, root string, shape runtimecheck.NodeShape) ([]string, error) {
	args := buildCommandArgs(shape.Manager)
	cmd := devCommandContext(ctx, shape.ManagerBinary, args...)
	cmd.Dir = root
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		return []string{"Build: " + shape.BuildCommandString()}, fmt.Errorf("node build failed: %v\n%s", err, out)
	}
	return []string{"Build: " + shape.BuildCommandString()}, nil
}

func singleLinePreview(text string) string {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "\n", "\\n")
	if len(text) > 160 {
		return text[:160] + "..."
	}
	return text
}

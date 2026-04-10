package runtimecheck

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var launcherTargetPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\$ROOT/([^"\s]+\.(?:mjs|js|cjs))`),
	regexp.MustCompile(`%ROOT%/([^"\r\n]+\.(?:mjs|js|cjs))`),
}

func diagnoseNode(project Project, nextValidate string) Diagnosis {
	shape := project.Node
	if shape.StructuralIssue != "" {
		return Diagnosis{
			Status: StatusBlocked,
			Reason: shape.StructuralIssue,
			Next:   []string{"plugin-kit-ai bootstrap .", nextValidate},
		}
	}
	if !shape.ManagerAvailable {
		return Diagnosis{
			Status: StatusBlocked,
			Reason: fmt.Sprintf("%s not found in PATH", shape.ManagerBinary),
			Next:   []string{"plugin-kit-ai bootstrap .", nextValidate},
		}
	}
	if !shape.Installed {
		return Diagnosis{
			Status: StatusNeedsBootstrap,
			Reason: fmt.Sprintf("%s install state is missing", shape.ManagerDisplay()),
			Next:   []string{"plugin-kit-ai bootstrap .", nextValidate},
		}
	}
	if shape.IsTypeScript && !shape.RuntimeTargetOK {
		return Diagnosis{
			Status: StatusNeedsBuild,
			Reason: fmt.Sprintf("built output %s is missing", shape.RuntimeTarget),
			Next:   []string{"plugin-kit-ai bootstrap .", nextValidate},
		}
	}
	if !shape.RuntimeTargetOK {
		return Diagnosis{
			Status: StatusBlocked,
			Reason: fmt.Sprintf("runtime target %s is missing", shape.RuntimeTarget),
			Next:   []string{nextValidate},
		}
	}
	return Diagnosis{
		Status: StatusReady,
		Reason: fmt.Sprintf("Node runtime is ready via %s", shape.ManagerDisplay()),
		Next:   []string{nextValidate},
	}
}

func inspectNode(root, entrypoint string) NodeShape {
	shape := NodeShape{
		Manager:         detectNodeManager(root),
		LauncherTarget:  detectNodeRuntimeTarget(root, entrypoint),
		OutputDir:       "dist",
		PackageJSONPath: "package.json",
	}
	if body, err := os.ReadFile(filepath.Join(root, "package.json")); err == nil {
		shape.PackageJSON = true
		var pkg packageJSON
		if json.Unmarshal(body, &pkg) == nil {
			shape.BuildScript = strings.TrimSpace(pkg.Scripts["build"])
			shape.PackageManager = strings.TrimSpace(pkg.PackageManager)
		}
	}
	if fileExists(filepath.Join(root, "tsconfig.json")) {
		shape.TSConfig = true
		shape.TSConfigPath = "tsconfig.json"
		if outDir := parseTSOutDir(root); outDir != "" {
			shape.OutputDir = outDir
		}
	}
	shape.ManagerBinary = string(shape.Manager)
	shape.ManagerAvailable = lookupBinary(shape.ManagerBinary)
	shape.UsesBuiltOutput = isBuiltOutputTarget(shape.LauncherTarget, shape.OutputDir)
	shape.RuntimeTarget = shape.LauncherTarget
	shape.RuntimeTargetOK = fileExists(filepath.Join(root, filepath.FromSlash(shape.RuntimeTarget)))
	shape.Installed = nodeInstallStatePresent(root)

	if !shape.PackageJSON {
		shape.StructuralIssue = "package.json is missing for node runtime"
		return shape
	}
	if shape.UsesBuiltOutput {
		if !shape.TSConfig {
			shape.StructuralIssue = fmt.Sprintf("launcher target %s points to built output but tsconfig.json is missing", shape.LauncherTarget)
			return shape
		}
		if shape.BuildScript == "" {
			shape.StructuralIssue = "TypeScript lane is missing package.json scripts.build"
			return shape
		}
		if !strings.HasPrefix(shape.LauncherTarget, shape.OutputDir+"/") {
			shape.StructuralIssue = fmt.Sprintf("launcher target %s is outside tsconfig outDir %s", shape.LauncherTarget, shape.OutputDir)
			return shape
		}
		shape.IsTypeScript = true
		return shape
	}
	if shape.TSConfig && shape.BuildScript != "" && strings.HasSuffix(shape.LauncherTarget, ".js") && !strings.HasPrefix(shape.LauncherTarget, shape.OutputDir+"/") {
		shape.StructuralIssue = fmt.Sprintf("launcher target %s is outside tsconfig outDir %s", shape.LauncherTarget, shape.OutputDir)
	}
	return shape
}

func (n NodeShape) ManagerDisplay() string {
	if n.Manager == "" {
		return "npm"
	}
	return string(n.Manager)
}

func (n NodeShape) BuildCommandString() string {
	switch n.Manager {
	case NodeManagerPNPM:
		return "pnpm run build"
	case NodeManagerYarn:
		return "yarn build"
	case NodeManagerBun:
		return "bun run build"
	default:
		return "npm run build"
	}
}

func detectNodeManager(root string) NodeManager {
	switch {
	case fileExists(filepath.Join(root, "bun.lock")) || fileExists(filepath.Join(root, "bun.lockb")):
		return NodeManagerBun
	case fileExists(filepath.Join(root, "pnpm-lock.yaml")):
		return NodeManagerPNPM
	case fileExists(filepath.Join(root, "yarn.lock")):
		return NodeManagerYarn
	default:
		return NodeManagerNPM
	}
}

func parseTSOutDir(root string) string {
	body, err := os.ReadFile(filepath.Join(root, "tsconfig.json"))
	if err != nil {
		return ""
	}
	var cfg tsConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		return ""
	}
	outDir := strings.TrimSpace(filepath.ToSlash(cfg.CompilerOptions.OutDir))
	if outDir == "" {
		return ""
	}
	return strings.TrimPrefix(outDir, "./")
}

func isBuiltOutputTarget(target, outDir string) bool {
	target = filepath.ToSlash(strings.TrimSpace(target))
	if outDir != "" && strings.HasPrefix(target, strings.Trim(strings.TrimSpace(outDir), "/")+"/") {
		return true
	}
	return strings.HasPrefix(target, "dist/") || strings.HasPrefix(target, "build/")
}

func nodeInstallStatePresent(root string) bool {
	if dirExists(filepath.Join(root, "node_modules")) {
		return true
	}
	return fileExists(filepath.Join(root, ".pnp.cjs")) || fileExists(filepath.Join(root, ".pnp.loader.mjs"))
}

func detectNodeRuntimeTarget(root, entrypoint string) string {
	body, err := os.ReadFile(launcherPath(root, entrypoint))
	if err != nil {
		return "src/main.mjs"
	}
	text := filepath.ToSlash(string(body))
	for _, pattern := range launcherTargetPatterns {
		matches := pattern.FindStringSubmatch(text)
		if len(matches) == 2 {
			return matches[1]
		}
	}
	return "src/main.mjs"
}

func YarnBerry(root string, packageManager string) bool {
	if fileExists(filepath.Join(root, ".yarnrc.yml")) {
		return true
	}
	if !strings.HasPrefix(packageManager, "yarn@") {
		return false
	}
	version := strings.TrimPrefix(packageManager, "yarn@")
	majorText := version
	if idx := strings.Index(majorText, "."); idx >= 0 {
		majorText = majorText[:idx]
	}
	major, err := strconv.Atoi(majorText)
	return err == nil && major >= 2
}

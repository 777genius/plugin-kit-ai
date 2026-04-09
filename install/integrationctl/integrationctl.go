package integrationctl

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/claude"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/codex"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/cursor"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/evidence"
	fsadapter "github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/fs"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/gemini"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/journal"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/jsonstate"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/locks"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/manifest"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/opencode"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/process"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/safemutate"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/source"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/workspacelock"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/usecase"
)

type AddParams struct {
	Source          string
	Targets         []string
	Scope           string
	AutoUpdate      *bool
	AdoptNewTargets string
	AllowPrerelease *bool
	DryRun          bool
}

type UpdateParams struct {
	Name   string
	All    bool
	DryRun bool
}

type RemoveParams struct {
	Name   string
	DryRun bool
}

type RepairParams struct {
	Name   string
	Target string
	DryRun bool
}

type ToggleParams struct {
	Name   string
	Target string
	DryRun bool
}

type SyncParams struct {
	DryRun bool
}

type Result struct {
	OperationID string
	Summary     string
	Report      domain.Report
}

type Report = domain.Report

func Add(ctx context.Context, p AddParams) (Result, error) {
	svc, err := newService()
	if err != nil {
		return Result{}, err
	}
	report, err := svc.Add(ctx, usecase.AddInput{
		Source:          p.Source,
		Targets:         p.Targets,
		Scope:           p.Scope,
		AutoUpdate:      p.AutoUpdate,
		AdoptNewTargets: p.AdoptNewTargets,
		AllowPrerelease: p.AllowPrerelease,
		DryRun:          p.DryRun,
	})
	if err != nil {
		return Result{}, err
	}
	return Result{OperationID: report.OperationID, Summary: report.Summary, Report: report}, nil
}

func Update(ctx context.Context, p UpdateParams) (Result, error) {
	svc, err := newService()
	if err != nil {
		return Result{}, err
	}
	var report domain.Report
	if p.All {
		report, err = svc.UpdateAll(ctx, p.DryRun)
	} else {
		report, err = svc.Update(ctx, usecase.NamedDryRunInput{Name: p.Name, DryRun: p.DryRun})
	}
	if err != nil {
		return Result{}, err
	}
	return Result{OperationID: report.OperationID, Summary: report.Summary, Report: report}, nil
}

func Remove(ctx context.Context, p RemoveParams) (Result, error) {
	svc, err := newService()
	if err != nil {
		return Result{}, err
	}
	report, err := svc.Remove(ctx, usecase.NamedDryRunInput{Name: p.Name, DryRun: p.DryRun})
	if err != nil {
		return Result{}, err
	}
	return Result{OperationID: report.OperationID, Summary: report.Summary, Report: report}, nil
}

func Repair(ctx context.Context, p RepairParams) (Result, error) {
	svc, err := newService()
	if err != nil {
		return Result{}, err
	}
	report, err := svc.Repair(ctx, usecase.NamedDryRunInput{Name: p.Name, Target: p.Target, DryRun: p.DryRun})
	if err != nil {
		return Result{}, err
	}
	return Result{OperationID: report.OperationID, Summary: report.Summary, Report: report}, nil
}

func Enable(ctx context.Context, p ToggleParams) (Result, error) {
	svc, err := newService()
	if err != nil {
		return Result{}, err
	}
	report, err := svc.Enable(ctx, usecase.NamedDryRunInput{Name: p.Name, Target: p.Target, DryRun: p.DryRun})
	if err != nil {
		return Result{}, err
	}
	return Result{OperationID: report.OperationID, Summary: report.Summary, Report: report}, nil
}

func Disable(ctx context.Context, p ToggleParams) (Result, error) {
	svc, err := newService()
	if err != nil {
		return Result{}, err
	}
	report, err := svc.Disable(ctx, usecase.NamedDryRunInput{Name: p.Name, Target: p.Target, DryRun: p.DryRun})
	if err != nil {
		return Result{}, err
	}
	return Result{OperationID: report.OperationID, Summary: report.Summary, Report: report}, nil
}

func Sync(ctx context.Context, p SyncParams) (Result, error) {
	svc, err := newService()
	if err != nil {
		return Result{}, err
	}
	report, err := svc.Sync(ctx, p.DryRun)
	if err != nil {
		return Result{}, err
	}
	return Result{OperationID: report.OperationID, Summary: report.Summary, Report: report}, nil
}

func List(ctx context.Context) (domain.Report, error) {
	svc, err := newService()
	if err != nil {
		return domain.Report{}, err
	}
	return svc.List(ctx)
}

func Doctor(ctx context.Context) (domain.Report, error) {
	svc, err := newService()
	if err != nil {
		return domain.Report{}, err
	}
	return svc.Doctor(ctx)
}

func ExitCodeFromErr(err error) int {
	return domain.ExitCodeFromErr(err)
}

func newService() (usecase.Service, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return usecase.Service{}, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return usecase.Service{}, err
	}
	repoRoot := discoverRepoRoot(cwd)
	fs := fsadapter.OS{}
	mutator := safemutate.OS{}
	service := usecase.Service{
		SourceResolver: source.Resolver{Runner: process.OS{}},
		ManifestLoader: manifest.Loader{},
		StateStore: jsonstate.Store{
			FS:   fs,
			Path: filepath.Join(home, ".plugin-kit-ai", "state.json"),
		},
		WorkspaceLock: workspacelock.Store{
			FS:   fs,
			File: filepath.Join(repoRoot, ".plugin-kit-ai.lock"),
		},
		LockManager: locks.FileLock{
			BaseDir: filepath.Join(home, ".plugin-kit-ai", "locks"),
		},
		Journal: journal.FileJournal{
			FS:      fs,
			BaseDir: filepath.Join(home, ".plugin-kit-ai", "operations"),
		},
		Evidence: evidence.Registry{
			FS:   fs,
			Path: filepath.Join(repoRoot, "docs", "generated", "integrationctl_evidence_registry.json"),
		},
		Adapters: map[domain.TargetID]ports.TargetAdapter{
			domain.TargetClaude:   claude.Adapter{Runner: process.OS{}, FS: fs, ProjectRoot: cwd, UserHome: home},
			domain.TargetCodex:    codex.Adapter{FS: fs, ProjectRoot: cwd, UserHome: home},
			domain.TargetGemini:   gemini.Adapter{Runner: process.OS{}, FS: fs, UserHome: home},
			domain.TargetCursor:   cursor.Adapter{FS: fs, SafeMutator: mutator, ProjectRoot: cwd, UserHome: home},
			domain.TargetOpenCode: opencode.Adapter{FS: fs, SafeMutator: mutator, ProjectRoot: cwd, UserHome: home},
		},
	}
	return service, nil
}

func discoverRepoRoot(start string) string {
	dir := start
	for {
		if fileExists(filepath.Join(dir, ".git")) && fileExists(filepath.Join(dir, "docs")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return start
		}
		dir = parent
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func NormalizeTargets(targets []string) []string {
	out := make([]string, 0, len(targets))
	for _, target := range targets {
		target = strings.ToLower(strings.TrimSpace(target))
		if target == "" {
			continue
		}
		out = append(out, target)
	}
	return out
}

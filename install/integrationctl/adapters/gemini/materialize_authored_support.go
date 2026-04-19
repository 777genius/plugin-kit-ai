package gemini

import (
	"path/filepath"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
)

func (a Adapter) copyAuthoredGeminiSupportFiles(sourceRoot, destRoot string) error {
	for _, pair := range [][2]string{
		{filepath.Join(sourceRoot, "plugin", "targets", "gemini", "commands"), filepath.Join(destRoot, "commands")},
		{filepath.Join(sourceRoot, "plugin", "targets", "gemini", "policies"), filepath.Join(destRoot, "policies")},
		{filepath.Join(sourceRoot, "plugin", "targets", "gemini", "agents"), filepath.Join(destRoot, "agents")},
		{filepath.Join(sourceRoot, "plugin", "skills"), filepath.Join(destRoot, "skills")},
	} {
		if err := copyDirIfExists(pair[0], pair[1]); err != nil {
			return domain.NewError(domain.ErrMutationApply, "copy Gemini authored directory", err)
		}
	}
	return copyAuthoredGeminiHooks(sourceRoot, destRoot)
}

func copyAuthoredGeminiHooks(sourceRoot, destRoot string) error {
	hooksSrc := filepath.Join(sourceRoot, "plugin", "targets", "gemini", "hooks", "hooks.json")
	if !fileExists(hooksSrc) {
		return nil
	}
	if err := copyFile(hooksSrc, filepath.Join(destRoot, "hooks", "hooks.json")); err != nil {
		return domain.NewError(domain.ErrMutationApply, "copy Gemini hooks", err)
	}
	return nil
}

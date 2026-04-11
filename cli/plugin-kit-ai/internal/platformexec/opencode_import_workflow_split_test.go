package platformexec

import (
	"testing"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func TestRequireOpenCodeImportedInputRejectsEmptyState(t *testing.T) {
	t.Parallel()
	if err := requireOpenCodeImportedInput(opencodeImportedState{}); err == nil {
		t.Fatal("expected missing input error")
	}
}

func TestBuildOpenCodeImportArtifactsAddsDefaultAgent(t *testing.T) {
	t.Parallel()
	artifacts, err := buildOpenCodeImportArtifacts(opencodeImportedState{
		artifacts:       map[string]pluginmodel.Artifact{},
		defaultAgent:    "planner",
		defaultAgentSet: true,
	})
	if err != nil {
		t.Fatalf("buildOpenCodeImportArtifacts: %v", err)
	}
	if _, ok := artifactBody(artifacts, "plugin/targets/opencode/default_agent.txt"); !ok {
		t.Fatalf("artifacts = %#v", artifacts)
	}
}

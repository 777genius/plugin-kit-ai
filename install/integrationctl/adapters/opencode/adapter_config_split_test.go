package opencode

import (
	"strings"
	"testing"
)

func TestExistingPluginRefsRejectsInvalidTuplePluginRef(t *testing.T) {
	t.Parallel()

	_, err := existingPluginRefs([]any{
		[]any{"@acme/plugin", "not-an-object"},
	})
	if err == nil {
		t.Fatal("expected invalid tuple plugin ref error")
	}
	if !strings.Contains(err.Error(), "tuple plugin ref options must be an object") {
		t.Fatalf("error = %v", err)
	}
}

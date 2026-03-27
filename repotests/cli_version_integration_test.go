package pluginkitairepo_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestPluginKitAIVersionCommand(t *testing.T) {
	bin := buildPluginKitAI(t)
	cmd := exec.Command(bin, "version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("plugin-kit-ai version: %v\n%s", err, out)
	}
	text := string(out)
	for _, want := range []string{"module:", "version:", "go:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("plugin-kit-ai version output missing %q:\n%s", want, text)
		}
	}
}

package process

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

func TestOSRunnerPreservesCommandOutputAndExitCode(t *testing.T) {
	if os.Getenv("AGENTPLUGINS_PROCESS_OUTPUT_HELPER") == "1" {
		fmt.Fprint(os.Stdout, "expected stdout")
		fmt.Fprint(os.Stderr, "expected stderr")
		os.Exit(7)
	}
	environment := append([]string(nil), os.Environ()...)
	environment = append(environment, "AGENTPLUGINS_PROCESS_OUTPUT_HELPER=1")
	result, err := (OS{}).Run(context.Background(), ports.Command{
		Argv: []string{os.Args[0], "-test.run=TestOSRunnerPreservesCommandOutputAndExitCode"}, Env: environment,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 7 || strings.TrimSpace(string(result.Stdout)) != "expected stdout" || strings.TrimSpace(string(result.Stderr)) != "expected stderr" {
		t.Fatalf("result = %+v", result)
	}
}

func TestOSRunnerBoundsStderrWhileRetainingDiagnosticEdges(t *testing.T) {
	if os.Getenv("AGENTPLUGINS_PROCESS_STDERR_HELPER") == "1" {
		fmt.Fprint(os.Stderr, "diagnostic-start:")
		fmt.Fprint(os.Stderr, strings.Repeat("x", 256*1024))
		fmt.Fprint(os.Stderr, ":diagnostic-end")
		os.Exit(9)
	}
	environment := append([]string(nil), os.Environ()...)
	environment = append(environment, "AGENTPLUGINS_PROCESS_STDERR_HELPER=1")
	result, err := (OS{}).Run(context.Background(), ports.Command{
		Argv: []string{os.Args[0], "-test.run=TestOSRunnerBoundsStderrWhileRetainingDiagnosticEdges"}, Env: environment,
	})
	if err != nil {
		t.Fatal(err)
	}
	diagnostic := string(result.Stderr)
	if result.ExitCode != 9 || len(result.Stderr) > 34*1024 || !strings.Contains(diagnostic, "diagnostic-start:") ||
		!strings.Contains(diagnostic, ":diagnostic-end") || !strings.Contains(diagnostic, "stderr bytes omitted") {
		t.Fatalf("bounded stderr result: exit=%d bytes=%d diagnostic=%q", result.ExitCode, len(result.Stderr), diagnostic)
	}
}

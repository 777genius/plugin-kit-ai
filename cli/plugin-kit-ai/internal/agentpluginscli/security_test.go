package agentpluginscli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/spf13/cobra"
)

func TestBlockingSecurityAssessmentRequiresExplicitNonInteractiveOverride(t *testing.T) {
	loaded := loadedPackage{security: testSecurityAssessment(1, 2)}
	command := securityTestCommand(&bytes.Buffer{}, strings.NewReader(""))
	err := authorizeSecurityAssessment(command, App{Terminal: false}, &options{format: "human"}, &loaded)
	if err == nil || !strings.Contains(err.Error(), "--accept-security-risk") || loaded.securityAuthorized {
		t.Fatalf("blocking assessment error=%v authorized=%t", err, loaded.securityAuthorized)
	}
	if err := authorizeSecurityAssessment(command, App{Terminal: false}, &options{format: "human", acceptSecurityRisk: true}, &loaded); err != nil || !loaded.securityAuthorized {
		t.Fatalf("explicit override error=%v authorized=%t", err, loaded.securityAuthorized)
	}
}

func TestBlockingSecurityAssessmentPromptsInteractiveUser(t *testing.T) {
	output := &bytes.Buffer{}
	loaded := loadedPackage{security: testSecurityAssessment(1, 0)}
	command := securityTestCommand(output, strings.NewReader("yes\n"))
	if err := authorizeSecurityAssessment(command, App{Terminal: true}, &options{format: "human"}, &loaded); err != nil {
		t.Fatal(err)
	}
	if !loaded.securityAuthorized || !strings.Contains(output.String(), "not a guarantee of safety") || !strings.Contains(output.String(), "Continue anyway") {
		t.Fatalf("unexpected security prompt output: %s", output.String())
	}
}

func TestWarningSecurityAssessmentDoesNotPrompt(t *testing.T) {
	output := &bytes.Buffer{}
	loaded := loadedPackage{security: testSecurityAssessment(0, 3)}
	command := securityTestCommand(output, strings.NewReader(""))
	if err := authorizeSecurityAssessment(command, App{Terminal: true}, &options{format: "human"}, &loaded); err != nil {
		t.Fatal(err)
	}
	if !loaded.securityAuthorized || strings.Contains(output.String(), "Continue anyway") {
		t.Fatalf("warning assessment prompted unexpectedly: %s", output.String())
	}
}

func securityTestCommand(output *bytes.Buffer, input *strings.Reader) *cobra.Command {
	command := &cobra.Command{}
	command.SetOut(output)
	command.SetIn(input)
	return command
}

func testSecurityAssessment(blocking, warnings int) *domain.SecurityAssessment {
	outcome := domain.SecurityNoBlockingFindings
	if blocking > 0 {
		outcome = domain.SecurityBlockingFindings
	} else if warnings > 0 {
		outcome = domain.SecurityWarnings
	}
	return &domain.SecurityAssessment{
		Scanner: domain.SecurityScanner{ID: domain.SecurityScannerID, Version: domain.SecurityScannerVersion},
		Outcome: outcome, Counts: domain.SecurityCounts{Blocking: blocking, Warnings: warnings, Total: blocking + warnings},
		Findings: []domain.SecurityFinding{{Code: "SEC330", Disposition: "blocking", Path: "mcp.json", Message: "test finding"}},
	}
}

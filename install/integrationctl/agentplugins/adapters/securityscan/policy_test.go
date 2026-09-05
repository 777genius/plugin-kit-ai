package securityscan

import "testing"

func TestDefaultPolicyDigestMatchesSignedSecurityIndexContract(t *testing.T) {
	const expected = "sha256:41d3640d31eac89e7b30777bbbe937b307908e4a8d7c29a3a0edca49cfe1d755"
	if actual := DefaultRequirement().Policy.Digest; actual != expected {
		t.Fatalf("policy digest = %q, want %q", actual, expected)
	}
}

package main

import "testing"

func TestNormalizePublicationRequestedTargetDefaultsToAll(t *testing.T) {
	t.Parallel()
	if got := normalizePublicationRequestedTarget("   "); got != "all" {
		t.Fatalf("target = %q", got)
	}
}

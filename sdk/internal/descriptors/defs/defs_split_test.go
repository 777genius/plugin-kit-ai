package defs

import "testing"

func TestEventsPreserveStableBucketOrder(t *testing.T) {
	t.Parallel()

	events := Events()
	if len(events) == 0 {
		t.Fatal("expected non-empty events registry")
	}
	if got := events[0]; got.Platform != "claude" || got.Event != "Stop" {
		t.Fatalf("first event = %s/%s", got.Platform, got.Event)
	}
	if got := events[len(events)-1]; got.Platform != "codex" || got.Event != "Notify" {
		t.Fatalf("last event = %s/%s", got.Platform, got.Event)
	}
}

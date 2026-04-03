package platformexec

import "testing"

func TestResolveRelativeRefRejectsEscapingPaths(t *testing.T) {
	t.Parallel()
	for _, ref := range []string{"../outside.json", "..", "/tmp/outside.json"} {
		if _, err := resolveRelativeRef(t.TempDir(), ref); err == nil {
			t.Fatalf("resolveRelativeRef(%q) succeeded, want error", ref)
		}
	}
}

func TestResolveRelativeRefNormalizesLocalPaths(t *testing.T) {
	t.Parallel()
	got, err := resolveRelativeRef(t.TempDir(), "./nested/../sidecar.json")
	if err != nil {
		t.Fatal(err)
	}
	if got != "sidecar.json" {
		t.Fatalf("resolved ref = %q, want %q", got, "sidecar.json")
	}
}

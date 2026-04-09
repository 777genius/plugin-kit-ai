package main

import "testing"

func TestRootCommandExposesIntegrationShortAliases(t *testing.T) {
	t.Helper()

	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"add"}, want: "add"},
		{args: []string{"update"}, want: "update"},
		{args: []string{"remove"}, want: "remove"},
		{args: []string{"repair"}, want: "repair"},
	}

	for _, tt := range tests {
		cmd, _, err := rootCmd.Find(tt.args)
		if err != nil {
			t.Fatalf("find %q: %v", tt.args[0], err)
		}
		if cmd == nil {
			t.Fatalf("find %q returned nil command", tt.args[0])
		}
		if cmd.Name() != tt.want {
			t.Fatalf("find %q resolved to %q", tt.args[0], cmd.Name())
		}
	}
}

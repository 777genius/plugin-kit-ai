package main

import (
	"os"

	pluginkitai "github.com/plugin-kit-ai/plugin-kit-ai/sdk"
	"github.com/plugin-kit-ai/plugin-kit-ai/sdk/claude"
)

func main() {
	app := pluginkitai.New(pluginkitai.Config{Name: "plugin-kit-ai-smoke"})
	app.Claude().OnStop(func(e *claude.StopEvent) *claude.Response {
		_ = e // smoke: allow stop
		return claude.Allow()
	})
	os.Exit(app.Run())
}

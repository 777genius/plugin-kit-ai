package main

import (
	"os"

	pluginkitai "github.com/plugin-kit-ai/plugin-kit-ai/sdk"
	"github.com/plugin-kit-ai/plugin-kit-ai/sdk/codex"
)

func main() {
	app := pluginkitai.New(pluginkitai.Config{Name: "my-plugin"})
	app.Codex().OnNotify(func(e *codex.NotifyEvent) *codex.Response {
		_ = e
		return codex.Continue()
	})
	os.Exit(app.Run())
}

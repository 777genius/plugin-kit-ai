package pluginkitai_test

import (
	pluginkitai "github.com/777genius/plugin-kit-ai/sdk"
	"github.com/777genius/plugin-kit-ai/sdk/claude"
	"github.com/777genius/plugin-kit-ai/sdk/codex"
)

func ExampleApp_Claude() {
	app := pluginkitai.New(pluginkitai.Config{Name: "demo"})
	app.Claude().OnStop(func(*claude.StopEvent) *claude.Response {
		return claude.Allow()
	})
	_ = app
}

func ExampleApp_Codex() {
	app := pluginkitai.New(pluginkitai.Config{Name: "demo"})
	app.Codex().OnNotify(func(*codex.NotifyEvent) *codex.Response {
		return codex.Continue()
	})
	_ = app
}

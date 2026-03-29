package codex

import (
	"encoding/json"

	internalcodex "github.com/777genius/plugin-kit-ai/sdk/internal/platforms/codex"
	"github.com/777genius/plugin-kit-ai/sdk/internal/runtime"
)

type NotifyEvent struct {
	Raw    json.RawMessage
	Client string
}

type Response struct{}

func Continue() *Response {
	return &Response{}
}

func (e *NotifyEvent) RawJSON() json.RawMessage {
	if e == nil {
		return nil
	}
	return e.Raw
}

func wrapNotify(fn func(*NotifyEvent) *Response) runtime.TypedHandler {
	return func(_ runtime.InvocationContext, v any) runtime.Handled {
		ev, ok := v.(*internalcodex.NotifyInput)
		if !ok {
			return runtime.Handled{Err: runtime.InternalHookTypeMismatch("codex Notify")}
		}
		_ = fn(&NotifyEvent{Raw: ev.Raw, Client: ev.Client})
		return runtime.Handled{Value: internalcodex.NotifyOutcome{}}
	}
}

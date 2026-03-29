package codex

func generatedRegistrarMarker() {}

func (r *Registrar) OnNotify(fn func(*NotifyEvent) *Response) {
	r.backend.Register("codex", "Notify", wrapNotify(fn))
}

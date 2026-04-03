package gemini

// OnAfterTool registers a handler for the gemini AfterTool.
func (r *Registrar) OnAfterTool(fn func(*AfterToolEvent) *AfterToolResponse) {
	r.backend.Register("gemini", "AfterTool", wrapAfterTool(fn))
}

// OnBeforeTool registers a handler for the gemini BeforeTool.
func (r *Registrar) OnBeforeTool(fn func(*BeforeToolEvent) *BeforeToolResponse) {
	r.backend.Register("gemini", "BeforeTool", wrapBeforeTool(fn))
}

// OnSessionEnd registers a handler for the gemini SessionEnd.
func (r *Registrar) OnSessionEnd(fn func(*SessionEndEvent) *SessionEndResponse) {
	r.backend.Register("gemini", "SessionEnd", wrapSessionEnd(fn))
}

// OnSessionStart registers a handler for the gemini SessionStart.
func (r *Registrar) OnSessionStart(fn func(*SessionStartEvent) *SessionStartResponse) {
	r.backend.Register("gemini", "SessionStart", wrapSessionStart(fn))
}

package claude

func generatedRegistrarMarker() {}

func (r *Registrar) OnConfigChange(fn func(*ConfigChangeEvent) *ConfigChangeResponse) {
	r.backend.Register("claude", "ConfigChange", wrapConfigChange(fn))
}

func (r *Registrar) OnNotification(fn func(*NotificationEvent) *NotificationResponse) {
	r.backend.Register("claude", "Notification", wrapNotification(fn))
}

func (r *Registrar) OnPermissionRequest(fn func(*PermissionRequestEvent) *PermissionRequestResponse) {
	r.backend.Register("claude", "PermissionRequest", wrapPermissionRequest(fn))
}

func (r *Registrar) OnPostToolUse(fn func(*PostToolUseEvent) *PostToolUseResponse) {
	r.backend.Register("claude", "PostToolUse", wrapPostToolUse(fn))
}

func (r *Registrar) OnPostToolUseFailure(fn func(*PostToolUseFailureEvent) *PostToolUseFailureResponse) {
	r.backend.Register("claude", "PostToolUseFailure", wrapPostToolUseFailure(fn))
}

func (r *Registrar) OnPreCompact(fn func(*PreCompactEvent) *PreCompactResponse) {
	r.backend.Register("claude", "PreCompact", wrapPreCompact(fn))
}

func (r *Registrar) OnPreToolUse(fn func(*PreToolUseEvent) *PreToolResponse) {
	r.backend.Register("claude", "PreToolUse", wrapPreToolUse(fn))
}

func (r *Registrar) OnSessionEnd(fn func(*SessionEndEvent) *SessionEndResponse) {
	r.backend.Register("claude", "SessionEnd", wrapSessionEnd(fn))
}

func (r *Registrar) OnSessionStart(fn func(*SessionStartEvent) *SessionStartResponse) {
	r.backend.Register("claude", "SessionStart", wrapSessionStart(fn))
}

func (r *Registrar) OnSetup(fn func(*SetupEvent) *SetupResponse) {
	r.backend.Register("claude", "Setup", wrapSetup(fn))
}

func (r *Registrar) OnStop(fn func(*StopEvent) *Response) {
	r.backend.Register("claude", "Stop", wrapStop(fn))
}

func (r *Registrar) OnSubagentStart(fn func(*SubagentStartEvent) *SubagentStartResponse) {
	r.backend.Register("claude", "SubagentStart", wrapSubagentStart(fn))
}

func (r *Registrar) OnSubagentStop(fn func(*SubagentStopEvent) *SubagentStopResponse) {
	r.backend.Register("claude", "SubagentStop", wrapSubagentStop(fn))
}

func (r *Registrar) OnTaskCompleted(fn func(*TaskCompletedEvent) *TaskCompletedResponse) {
	r.backend.Register("claude", "TaskCompleted", wrapTaskCompleted(fn))
}

func (r *Registrar) OnTeammateIdle(fn func(*TeammateIdleEvent) *TeammateIdleResponse) {
	r.backend.Register("claude", "TeammateIdle", wrapTeammateIdle(fn))
}

func (r *Registrar) OnUserPromptSubmit(fn func(*UserPromptEvent) *UserPromptResponse) {
	r.backend.Register("claude", "UserPromptSubmit", wrapUserPromptSubmit(fn))
}

func (r *Registrar) OnWorktreeCreate(fn func(*WorktreeCreateEvent) *WorktreeCreateResponse) {
	r.backend.Register("claude", "WorktreeCreate", wrapWorktreeCreate(fn))
}

func (r *Registrar) OnWorktreeRemove(fn func(*WorktreeRemoveEvent) *WorktreeRemoveResponse) {
	r.backend.Register("claude", "WorktreeRemove", wrapWorktreeRemove(fn))
}

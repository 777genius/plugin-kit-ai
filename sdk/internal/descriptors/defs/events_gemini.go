package defs

func geminiEvents() []EventDescriptor {
	events := make([]EventDescriptor, 0, len(geminiSessionEvents())+len(geminiModelEvents())+len(geminiAgentToolEvents()))
	events = append(events, geminiSessionEvents()...)
	events = append(events, geminiModelEvents()...)
	events = append(events, geminiAgentToolEvents()...)
	return events
}

package platformexec

func importOpenCodeConfigArtifacts(state *opencodeImportedState, importedConfig importedOpenCodeConfig, configDisplayPath string) error {
	commandArtifacts, remainingCommands, commandWarnings, err := importedOpenCodeInlineCommandArtifacts(importedConfig.Commands, configDisplayPath)
	if err != nil {
		return err
	}
	agentArtifacts, remainingAgents, agentWarnings, err := importedOpenCodeInlineAgentArtifacts(importedConfig.Agents, configDisplayPath)
	if err != nil {
		return err
	}
	state.warnings = append(state.warnings, commandWarnings...)
	state.warnings = append(state.warnings, agentWarnings...)
	state.addArtifacts(commandArtifacts...)
	state.addArtifacts(agentArtifacts...)
	if len(remainingCommands) > 0 {
		if importedConfig.Extra == nil {
			importedConfig.Extra = map[string]any{}
		}
		importedConfig.Extra["command"] = remainingCommands
	}
	if len(remainingAgents) > 0 {
		if importedConfig.Extra == nil {
			importedConfig.Extra = map[string]any{}
		}
		importedConfig.Extra["agent"] = remainingAgents
	}
	state.mergeConfig(importedConfig)
	return nil
}

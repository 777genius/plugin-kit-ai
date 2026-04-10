package platformexec

func importOpenCodeWorkspaceArtifacts(state *opencodeImportedState, cfg opencodeScopeConfig) error {
	if err := importOpenCodeThemeArtifacts(state, cfg); err != nil {
		return err
	}
	if err := importOpenCodeToolArtifactsIntoState(state, cfg); err != nil {
		return err
	}
	if err := importOpenCodeCommandDirectory(state, cfg); err != nil {
		return err
	}
	if err := importOpenCodeAgentDirectory(state, cfg); err != nil {
		return err
	}
	if err := importOpenCodeSkillDirectory(state, cfg); err != nil {
		return err
	}
	if err := importOpenCodePluginDirectory(state, cfg); err != nil {
		return err
	}
	return importOpenCodePackageJSON(state, cfg)
}

package platformexec

func importOpenCodeScope(state *opencodeImportedState, cfg opencodeScopeConfig) error {
	importedConfig, configDisplayPath, warnings, ok, err := readImportedOpenCodeConfigFromDir(cfg.root, cfg.displayConfigRoot)
	if err != nil {
		return err
	}
	state.warnings = append(state.warnings, warnings...)
	if ok {
		if err := importOpenCodeConfigArtifacts(state, importedConfig, configDisplayPath); err != nil {
			return err
		}
		state.hasInput = true
	}
	if err := importOpenCodeWorkspaceArtifacts(state, cfg); err != nil {
		return err
	}
	return nil
}

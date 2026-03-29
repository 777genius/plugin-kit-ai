.PHONY: test test-required test-plugin-manifest-workflow test-install-compat test-extended test-polyglot-smoke test-live test-live-cli test-install-live test-opencode-live test-e2e-live generated-check release-gate release-rehearsal build-plugin-kit-ai vet

GOCACHE ?= /tmp/plugin-kit-ai-gocache
export GOCACHE

EXTENDED_TEST_ARGS ?=

test:
	$(MAKE) test-required

test-required:
	go test ./...

test-plugin-manifest-workflow:
	go test -count=1 -run 'TestPluginKitAI(ValidateWarnsButSucceedsOnExtraPluginYAMLFields|ValidateStrictFailsOnWarningsThenNormalizeFixesThem|ImportPrintsWarningsForIgnoredAssets|MigrationFixtures_RoundTripToStrictValidation)$$' ./repotests

test-install-compat:
	go test -count=1 -run '^TestPluginKitAIInstall_' ./repotests

test-extended:
	go test -count=1 -run '^TestClaudeCLIHooks$$' ./repotests $(EXTENDED_TEST_ARGS)
	go test -count=1 -run '^TestCodexCLINotify$$' ./repotests $(EXTENDED_TEST_ARGS)

test-polyglot-smoke:
	go test -count=1 -run 'TestRenderTemplate_(PythonLauncherWindowsFallbackOrder|ShellLauncherWindowsRequiresBash)$$' ./cli/plugin-kit-ai/internal/scaffold
	go test -count=1 -run 'Test(FindPython_UsesPlatformAwareLookupOrder|Validate_ManifestProject_WindowsCmdLauncherAccepted|Validate_ManifestProject_ShellRequiresBashOnWindows|ValidateNodeRuntimeTarget_MissingBuiltOutputShowsRecoveryGuidance|ValidateRuntimeTargetExecutable_NonExecutableScriptFails|ShellLauncherPassthrough)$$' ./cli/plugin-kit-ai/internal/validate
	go test -count=1 -run 'TestPluginService(DoctorReadyNeedsBootstrapNeedsBuildAndBlocked|DoctorPoetryManagerOwnedEnvIsReady|BootstrapPythonCreatesVenvAndInstallsRequirements|BootstrapPoetryReportsManagerOwnedEnv|BootstrapNodePNPMTypeScriptRunsInstallAndBuild|ExportPythonBundleExcludesProjectVenv|ExportShellBundlePreservesScripts|ExportRejectsGoRuntime|BundleInstallInstallsPythonBundleIntoDestination|BundleInstallRejectsRemoteURL|BundleFetchURLInstallsPythonBundleWithExplicitChecksum|BundleFetchURLUsesSidecarChecksum|BundleFetchURLRejectsHTTP|BundleFetchURLFailsChecksumMismatch|BundleFetchGitHubInstallsNodeBundleFromChecksumsTxt|BundleFetchGitHubFallsBackToSidecarChecksum|BundleFetchGitHubUsesLatestRelease|BundleFetchGitHubRejectsMetadataMismatch|BundlePublishCreatesPublishedReleaseByDefault|BundlePublishCreatesDraftReleaseWhenRequested|BundlePublishPromotesExistingDraftReleaseToPublished|BundlePublishReusesExistingDraftReleaseWhenRequested|BundlePublishReusesExistingPublishedReleaseWithForce|BundlePublishFailsWhenAssetExistsWithoutForce|BundlePublishRejectsShellRuntime)$$|TestSelectBundleReleaseAsset(RejectsAmbiguous|UsesPlatformRuntime|UsesExactAssetName)$$' ./cli/plugin-kit-ai/internal/app
	go test -count=1 -run 'TestBundle(Install(HelpIncludesLocalTarballLanguage|WritesRunnerOutput)|Fetch(HelpIncludesURLAndGitHubLanguage|WritesRunnerOutput)|Publish(HelpIncludesGitHubLanguage|WritesRunnerOutput))$$' ./cli/plugin-kit-ai/cmd/plugin-kit-ai
	go test -count=1 -run 'TestPluginKitAI(Init(GoRuntimeLauncherFlow|PythonRuntimeLauncherFlow|PythonRuntimeWithRequirementsDoctorBootstrapFlow|PythonRuntimeBrokenVenvFailsValidate|ShellRuntimeLauncherFlow|ShellRuntimeNonExecutableTargetFailsValidate|NodeRuntimeSupportsTypeScriptBuildThroughLauncher|NodeRuntimePNPMDoctorBootstrapFlow|NodeRuntimeMissingBuiltOutputFailsValidate)|RuntimeABIPassthrough|PythonLauncherPrefersProjectVenvOnWindows)$$' ./repotests
	go test -count=1 -run 'TestPluginKitAI(BootstrapScriptInstallsLatestRelease|BootstrapScriptSupportsExplicitVersion|BootstrapScriptRejectsChecksumMismatch|InitExtras(PythonEmitsBundleReleaseWorkflow|NodeTypeScriptEmitsBundleReleaseWorkflow))$$|TestSetupPluginKitAIActionUsesInstallScript$$' ./repotests
	go test -count=1 -run 'Test(HomebrewFormulaGeneratorFromChecksums|NPMCLIPackageContractFiles|PythonCLIPackageContractFiles|StarterRepos_(LayoutAndReadmesStayAligned|Smoke)|StarterTemplate(SyncContractFilesStayAligned|SyncScriptSupportsLocalMirror|RepoLinksResolveToCurrentOwnerNaming)|ReleaseSurface_MakefileDocsAndWorkflowsStayAligned|ContractClarity_RuntimeMetadataAndDocsStayAligned)$$' ./repotests
	go test -count=1 -run 'TestPluginKitAIExport(PythonRequirementsBundleFlow|NodeTypeScriptBundleFlow|ShellBundleFlow)|TestPluginKitAIBundleInstall(PythonRequirementsFlow|NodeTypeScriptFlow|ClaudeNodeTypeScriptFlow)$$' ./repotests
	go test -count=1 -run 'TestPluginKitAIBundle(Fetch(URL(PythonRequirementsFlow|ClaudeNodeTypeScriptFlow)|GitHub(ClaudeNodeTypeScriptFlow|LatestClaudeNodeTypeScriptFlow))|PublishFetch(PythonRequirementsFlow|ClaudeNodeTypeScriptFlow))$$' ./repotests
	go test -count=1 -run '^TestNPMCLIPackage' ./repotests
	go test -count=1 -run '^TestPythonCLIPackage' ./repotests

# Live E2E: real GitHub + real claude-notifications-go release (needs network). Optional: GITHUB_TOKEN.
# Package is ./repotests (tests moved out of repo root).
test-live: test-e2e-live

test-live-cli:
	go test -count=1 -run 'TestClaudeHooks_LiveHaikuLow' ./repotests $(EXTENDED_TEST_ARGS)

test-install-live:
	PLUGIN_KIT_AI_E2E_LIVE=1 go test -count=1 -timeout=15m -run '^TestLiveInstall_' ./repotests

test-opencode-live:
	PLUGIN_KIT_AI_ENABLE_OPENCODE_SMOKE=1 go test -count=1 -run '^TestOpenCodeLoaderSmoke$$' ./repotests $(EXTENDED_TEST_ARGS)

test-e2e-live: test-install-live

# Root module is workspace-only; submodules are vetted explicitly.
vet:
	go vet ./...
	cd cli/plugin-kit-ai && go vet ./...
	cd install/plugininstall && go vet ./...
	cd sdk && go vet ./...

generated-check:
	bash ./scripts/check-generated-sync.sh

release-gate:
	$(MAKE) test-required
	$(MAKE) vet
	$(MAKE) generated-check

release-rehearsal: release-gate
	$(MAKE) test-install-compat
	$(MAKE) test-polyglot-smoke
	@echo "Release rehearsal deterministic checks complete. Record extended/live evidence (including OpenCode smoke when refreshing that stable boundary), audit updates, release notes draft, and any waiver notes tied to the candidate commit SHA."

build-plugin-kit-ai:
	go build -o bin/plugin-kit-ai ./cli/plugin-kit-ai/cmd/plugin-kit-ai

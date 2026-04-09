.PHONY: test test-required test-plugin-manifest-workflow test-install-compat test-extended test-polyglot-smoke test-live test-live-cli test-install-live test-gemini-live test-gemini-runtime test-gemini-runtime-live test-opencode-live test-opencode-cli-live test-opencode-tools-live test-cursor-live test-portable-mcp-live test-context7-live test-chrome-devtools-live test-e2e-live generated-check version-sync-check removed-contract-boundary-check release-gate release-rehearsal build-plugin-kit-ai vet

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
	go test -count=1 -run 'Test(HomebrewFormulaGeneratorFromChecksums|NPMCLIPackageContractFiles|PythonCLIPackageContractFiles|NPMRuntimePackage(ContractFiles|ClaudeAndCodexSmoke)|PythonRuntimePackage(ContractFiles|ClaudeAndCodexSmoke)|StarterRepos_(LayoutAndReadmesStayAligned|Smoke)|StarterTemplate(SyncContractFilesStayAligned|SyncScriptSupportsLocalMirror|RepoLinksResolveToCurrentOwnerNaming)|ReleaseSurface_MakefileDocsAndWorkflowsStayAligned|ContractClarity_RuntimeMetadataAndDocsStayAligned)$$' ./repotests
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

test-gemini-live:
	PLUGIN_KIT_AI_RUN_GEMINI_CLI=1 go test -count=1 -run '^TestGeminiCLIExtensionLink$$' ./repotests $(EXTENDED_TEST_ARGS)

test-gemini-runtime:
	go test -count=1 ./sdk/... $(EXTENDED_TEST_ARGS)
	go test -count=1 -run 'TestInitRunner_geminiGoRuntimeStarter' ./cli/plugin-kit-ai/internal/app $(EXTENDED_TEST_ARGS)
	go test -count=1 -run 'TestInspectTextShowsLauncherAndGeminiGuidance' ./cli/plugin-kit-ai/cmd/plugin-kit-ai $(EXTENDED_TEST_ARGS)
	go test -count=1 -run 'TestPluginKitAIInitGeminiGoRuntimeLauncherFlow|TestGeneratedConfigCanaries_GeminiRuntimeContract|TestGeminiE2ETracePreservesOriginalRequestName|TestGeminiE2ETraceCapturesModelAndToolSelectionPayloads|TestGeminiE2ETraceCapturesRuntimeLifecycleHooks|TestGeminiE2ETraceCapturesRuntimeControlSemantics|TestGeminiE2ETraceCapturesRuntimeTransformSemantics|TestContractClarity_GeminiRuntimeDocsStayAligned' ./repotests $(EXTENDED_TEST_ARGS)

test-gemini-runtime-live:
	PLUGIN_KIT_AI_RUN_GEMINI_RUNTIME_LIVE=1 go test -count=1 -run '^TestGeminiCLIRuntime(Hooks|BeforeToolDeny|BeforeModelDeny|DisableAllTools|AfterModelReplaceResponse|AfterAgentRetry|RewriteToolInput)$$' ./repotests $(EXTENDED_TEST_ARGS)

test-opencode-live:
	PLUGIN_KIT_AI_ENABLE_OPENCODE_SMOKE=1 go test -count=1 -run '^TestOpenCodeLoaderSmoke$$' ./repotests $(EXTENDED_TEST_ARGS)

test-opencode-cli-live:
	PLUGIN_KIT_AI_RUN_OPENCODE_CLI=1 go test -count=1 -run '^TestOpenCodeCLI' ./repotests $(EXTENDED_TEST_ARGS)

test-opencode-tools-live:
	PLUGIN_KIT_AI_ENABLE_OPENCODE_SMOKE=1 go test -count=1 -run '^TestOpenCodeStandaloneToolsSmoke$$' ./repotests $(EXTENDED_TEST_ARGS)

test-cursor-live:
	PLUGIN_KIT_AI_RUN_CURSOR_CLI=1 go test -count=1 -run '^TestCursorCLI' ./repotests $(EXTENDED_TEST_ARGS)

test-portable-mcp-live:
	PLUGIN_KIT_AI_RUN_PORTABLE_MCP_LIVE=1 go test -count=1 -run '^TestPortableMCPLiveAcrossConsoleAgents$$' ./repotests $(EXTENDED_TEST_ARGS)

test-context7-live:
	PLUGIN_KIT_AI_RUN_CONTEXT7_LIVE=1 go test -count=1 -run '^TestContext7CatalogLiveAcrossInstalledAgents$$' ./repotests $(EXTENDED_TEST_ARGS)

test-chrome-devtools-live:
	PLUGIN_KIT_AI_RUN_CHROME_DEVTOOLS_LIVE=1 go test -count=1 -run '^TestChromeDevtoolsCatalogLiveAcrossInstalledAgents$$' ./repotests $(EXTENDED_TEST_ARGS)

test-vercel-live:
	PLUGIN_KIT_AI_RUN_VERCEL_LIVE=1 go test -count=1 -run '^TestVercelCatalogLiveAcrossInstalledAgents$$' ./repotests $(EXTENDED_TEST_ARGS)

test-sentry-live:
	PLUGIN_KIT_AI_RUN_SENTRY_LIVE=1 go test -count=1 -run '^TestSentryCatalogLiveAcrossInstalledAgents$$' ./repotests $(EXTENDED_TEST_ARGS)

test-stripe-live:
	PLUGIN_KIT_AI_RUN_STRIPE_LIVE=1 go test -count=1 -run '^TestStripeCatalogLiveAcrossInstalledAgents$$' ./repotests $(EXTENDED_TEST_ARGS)

test-slack-live:
	PLUGIN_KIT_AI_RUN_SLACK_LIVE=1 go test -count=1 -run '^TestSlackCatalogLiveAcrossSupportedAgents$$' ./repotests $(EXTENDED_TEST_ARGS)

test-e2e-live: test-install-live

# Root module is workspace-only; submodules are vetted explicitly.
vet:
	go vet ./...
	cd cli/plugin-kit-ai && go vet ./...
	cd install/plugininstall && go vet ./...
	cd sdk && go vet ./...

generated-check:
	bash ./scripts/check-generated-sync.sh
	$(MAKE) version-sync-check
	$(MAKE) removed-contract-boundary-check

version-sync-check:
	bash ./scripts/check-version-sync.sh

removed-contract-boundary-check:
	bash ./scripts/check-removed-contract-boundary.sh

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

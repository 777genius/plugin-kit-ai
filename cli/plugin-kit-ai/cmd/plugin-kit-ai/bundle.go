package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/app"
	"github.com/spf13/cobra"
)

type bundleRunner interface {
	BundleInstall(app.PluginBundleInstallOptions) (app.PluginBundleInstallResult, error)
	BundleFetch(context.Context, app.PluginBundleFetchOptions) (app.PluginBundleFetchResult, error)
	BundlePublish(context.Context, app.PluginBundlePublishOptions) (app.PluginBundlePublishResult, error)
}

var (
	bundleInstallDest  string
	bundleInstallForce bool

	bundleFetchURL           string
	bundleFetchDest          string
	bundleFetchSHA256        string
	bundleFetchAssetName     string
	bundleFetchPlatform      string
	bundleFetchRuntime       string
	bundleFetchTag           string
	bundleFetchGitHubToken   string
	bundleFetchGitHubAPIBase string
	bundleFetchForce         bool
	bundleFetchLatest        bool

	bundlePublishPlatform      string
	bundlePublishRepo          string
	bundlePublishTag           string
	bundlePublishDraft         bool
	bundlePublishGitHubToken   string
	bundlePublishGitHubAPIBase string
	bundlePublishForce         bool
)

var bundleCmd = newBundleCmd(pluginService)

func newBundleCmd(runner bundleRunner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bundle",
		Short: "Bundle tooling for exported interpreted-runtime handoff archives",
	}
	cmd.AddCommand(newBundleInstallCmd(runner))
	cmd.AddCommand(newBundleFetchCmd(runner))
	cmd.AddCommand(newBundlePublishCmd(runner))
	return cmd
}

func newBundleInstallCmd(runner bundleRunner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install <bundle.tar.gz>",
		Short: "Install a local exported Python/Node bundle into a destination directory",
		Long: `Install a local .tar.gz bundle created by plugin-kit-ai export into a destination directory.

This stable local handoff surface only supports local exported Python/Node bundles for codex-runtime or claude.
It unpacks bundle contents safely, prints next steps, and does not extend the binary-only plugin-kit-ai install flow.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := runner.BundleInstall(app.PluginBundleInstallOptions{
				Archive: args[0],
				Dest:    bundleInstallDest,
				Force:   bundleInstallForce,
			})
			if err != nil {
				return err
			}
			for _, line := range result.Lines {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&bundleInstallDest, "dest", "", "destination directory for unpacked bundle contents")
	cmd.Flags().BoolVarP(&bundleInstallForce, "force", "f", false, "overwrite an existing destination directory")
	_ = cmd.MarkFlagRequired("dest")
	return cmd
}

func newBundleFetchCmd(runner bundleRunner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fetch [owner/repo]",
		Short: "Fetch and install a remote exported Python/Node bundle into a destination directory",
		Long: `Fetch a remote exported Python/Node bundle and install it into a destination directory.

Use either a direct HTTPS bundle URL with --url or a GitHub release reference as owner/repo plus --tag or --latest.
This beta remote handoff surface is intentionally separate from the binary-only plugin-kit-ai install flow.`,
		Args: validateBundleFetchArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			token := strings.TrimSpace(bundleFetchGitHubToken)
			if token == "" {
				token = strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
			}
			ref := ""
			if len(args) == 1 {
				ref = args[0]
			}
			result, err := runner.BundleFetch(ctx, app.PluginBundleFetchOptions{
				URL:           bundleFetchURL,
				Ref:           ref,
				Tag:           bundleFetchTag,
				Latest:        bundleFetchLatest,
				Dest:          bundleFetchDest,
				SHA256:        bundleFetchSHA256,
				AssetName:     bundleFetchAssetName,
				Platform:      bundleFetchPlatform,
				Runtime:       bundleFetchRuntime,
				GitHubToken:   token,
				GitHubAPIBase: bundleFetchGitHubAPIBase,
				Force:         bundleFetchForce,
			})
			if err != nil {
				return err
			}
			for _, line := range result.Lines {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&bundleFetchURL, "url", "", "direct HTTPS URL to an exported .tar.gz bundle")
	cmd.Flags().StringVar(&bundleFetchDest, "dest", "", "destination directory for unpacked bundle contents")
	cmd.Flags().StringVar(&bundleFetchSHA256, "sha256", "", "expected SHA256 for URL mode; overrides .sha256 sidecar lookup")
	cmd.Flags().StringVar(&bundleFetchAssetName, "asset-name", "", "specific GitHub release bundle asset name to install")
	cmd.Flags().StringVar(&bundleFetchPlatform, "platform", "", "bundle platform hint for GitHub mode (codex-runtime or claude)")
	cmd.Flags().StringVar(&bundleFetchRuntime, "runtime", "", "bundle runtime hint for GitHub mode (python or node)")
	cmd.Flags().StringVar(&bundleFetchTag, "tag", "", "GitHub release tag for bundle selection")
	cmd.Flags().BoolVar(&bundleFetchLatest, "latest", false, "install from the latest GitHub release instead of --tag")
	cmd.Flags().StringVar(&bundleFetchGitHubToken, "github-token", "", "GitHub token (optional; default from GITHUB_TOKEN env)")
	cmd.Flags().StringVar(&bundleFetchGitHubAPIBase, "github-api-base", "", "GitHub API base URL override (for tests or GitHub Enterprise)")
	cmd.Flags().BoolVarP(&bundleFetchForce, "force", "f", false, "overwrite an existing destination directory")
	_ = cmd.MarkFlagRequired("dest")
	return cmd
}

func validateBundleFetchArgs(cmd *cobra.Command, args []string) error {
	hasURL := strings.TrimSpace(bundleFetchURL) != ""
	switch {
	case hasURL && len(args) != 0:
		return fmt.Errorf("bundle fetch accepts no positional args with --url")
	case !hasURL && len(args) != 1:
		return fmt.Errorf("bundle fetch requires --url or owner/repo")
	default:
		return nil
	}
}

func newBundlePublishCmd(runner bundleRunner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "publish [path]",
		Short: "Publish an exported Python/Node bundle to GitHub Releases",
		Long: `Publish an exported Python/Node bundle to GitHub Releases.

This beta producer-side handoff surface exports a bundle, creates a published release by default,
uses --draft to keep the release as draft, uploads the bundle plus a sibling .sha256 asset,
and remains separate from the binary-only plugin-kit-ai install flow.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			token := strings.TrimSpace(bundlePublishGitHubToken)
			if token == "" {
				token = strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
			}
			result, err := runner.BundlePublish(ctx, app.PluginBundlePublishOptions{
				Root:          root,
				Platform:      bundlePublishPlatform,
				Repo:          bundlePublishRepo,
				Tag:           bundlePublishTag,
				Draft:         bundlePublishDraft,
				GitHubToken:   token,
				GitHubAPIBase: bundlePublishGitHubAPIBase,
				Force:         bundlePublishForce,
			})
			if err != nil {
				return err
			}
			for _, line := range result.Lines {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&bundlePublishPlatform, "platform", "", "target platform to export and publish (codex-runtime or claude)")
	cmd.Flags().StringVar(&bundlePublishRepo, "repo", "", "GitHub owner/repo that will receive the bundle assets")
	cmd.Flags().StringVar(&bundlePublishTag, "tag", "", "GitHub release tag to reuse or create")
	cmd.Flags().BoolVar(&bundlePublishDraft, "draft", false, "keep the target release as draft instead of published")
	cmd.Flags().StringVar(&bundlePublishGitHubToken, "github-token", "", "GitHub token (optional; default from GITHUB_TOKEN env)")
	cmd.Flags().StringVar(&bundlePublishGitHubAPIBase, "github-api-base", "", "GitHub API base URL override (for tests or GitHub Enterprise)")
	cmd.Flags().BoolVarP(&bundlePublishForce, "force", "f", false, "replace existing bundle assets with the same name")
	_ = cmd.MarkFlagRequired("platform")
	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("tag")
	_ = cmd.Flags().MarkHidden("github-api-base")
	return cmd
}

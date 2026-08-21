package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/777genius/plugin-kit-ai/cli/internal/agentpluginscli"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/dirswap"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/locks"
	processadapter "github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/process"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/clientdetect"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/directoryv1"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/loader"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/processlock"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/sourceacquisition"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/specregistry"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/statemigration"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/statev2"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	clientplanner "github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/planner"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/providers"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/transaction"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/usecase"
	"golang.org/x/term"
)

var (
	version                   = "0.1.0-development"
	defaultDirectoryOrigin    = "https://777genius.github.io/universal-agent-plugins/registry/schemas/1/"
	defaultDirectoryKeyID     = "uap-directory-2026-01"
	defaultDirectoryPublicKey = "HalXARjat+v3ylTPLMAnvuavRo4ZfrF+DbWwsjlp2bI="
	directoryClientFactory    = newDirectoryClient
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "agentplugins:", err)
		os.Exit(1)
	}
}

func run() error {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return fmt.Errorf("resolve user home: %w", err)
	}
	dataRoot, err := agentpluginsHome()
	if err != nil {
		return err
	}
	directoryClient, err := directoryClientFactory(dataRoot)
	if err != nil {
		return err
	}
	registry, err := specregistry.New()
	if err != nil {
		return err
	}
	packageLoader := loader.Loader{Registry: registry}
	runner := processadapter.OS{}
	v2Store := statev2.Store{Path: filepath.Join(dataRoot, "state-v2.json")}
	directoryManager := dirswap.Manager{JournalDir: filepath.Join(dataRoot, "operations-v2")}
	mutationLock := processlock.Lock{Path: filepath.Join(dataRoot, "mutation.lock")}
	stager := providers.Stager{}
	activator := providers.Activator{Runner: runner}
	planner := clientplanner.Planner{ManagedRoot: filepath.Join(dataRoot, "managed"), Detected: map[domain.ClientID]domain.DetectedClient{}}
	lifecycle := usecase.Service{
		StateStore: v2Store, Planner: planner, Targets: planner, Stager: stager, Activator: activator,
		Lock: mutationLock, Kernel: transaction.Kernel{StateStore: v2Store, Directory: directoryManager},
		NativeObserver: providers.NativeIdentityObserver{Stager: stager, Runner: runner}, PluginData: providers.PluginDataManager{Base: filepath.Join(dataRoot, "plugin-data")},
	}
	legacyStatePath := filepath.Join(home, ".plugin-kit-ai", "state.json")
	migrator := statemigration.Migrator{
		LegacyPath:     legacyStatePath,
		V2Store:        v2Store,
		Lock:           mutationLock,
		RecoverJournal: lifecycle.Kernel.Recover,
	}
	app := agentpluginscli.App{
		Version:             version,
		UserHome:            home,
		ManagedRoot:         filepath.Join(dataRoot, "managed"),
		StateStore:          v2Store,
		StateMigrator:       &migrator,
		LegacyLifecycle:     agentpluginscli.NewLegacyLifecycle(legacyStatePath),
		LegacyStateLock:     locks.FileLock{BaseDir: filepath.Join(home, ".plugin-kit-ai", "locks")},
		Detector:            clientdetect.NewOS(home),
		DirectoryClient:     directoryClient,
		SourceAcquirer:      lazySourceAcquirer{dataRoot: dataRoot, acquirer: sourceacquisition.Acquirer{TempRoot: dataRoot}},
		PackageLoader:       packageLoader,
		NativePackageLoader: loader.OpenAILoader{Loader: packageLoader},
		Lifecycle:           lifecycle,
		Input:               os.Stdin,
		Output:              os.Stdout,
		ErrorOutput:         os.Stderr,
		Terminal:            term.IsTerminal(int(os.Stdin.Fd())),
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return agentpluginscli.NewRoot(app).ExecuteContext(ctx)
}

// lazySourceAcquirer creates the private temporary root only once a validated
// command actually begins source acquisition. CLI construction and rejected
// flags/scopes therefore leave AGENTPLUGINS_HOME untouched.
type lazySourceAcquirer struct {
	dataRoot string
	acquirer sourceacquisition.Acquirer
}

func (acquirer lazySourceAcquirer) prepare() error {
	if err := os.MkdirAll(acquirer.dataRoot, 0o700); err != nil {
		return fmt.Errorf("create agentplugins data directory: %w", err)
	}
	return nil
}

func (acquirer lazySourceAcquirer) AcquireLocal(ctx context.Context, source string) (domain.PackageSnapshot, error) {
	if err := acquirer.prepare(); err != nil {
		return domain.PackageSnapshot{}, err
	}
	return acquirer.acquirer.AcquireLocal(ctx, source)
}

func (acquirer lazySourceAcquirer) AcquireGitHub(ctx context.Context, repository, revision, subpath string) (domain.PackageSnapshot, error) {
	if err := acquirer.prepare(); err != nil {
		return domain.PackageSnapshot{}, err
	}
	return acquirer.acquirer.AcquireGitHub(ctx, repository, revision, subpath)
}

func (acquirer lazySourceAcquirer) AcquireGitHubVerified(ctx context.Context, repository, revision, subpath, digest string) (domain.PackageSnapshot, error) {
	if err := acquirer.prepare(); err != nil {
		return domain.PackageSnapshot{}, err
	}
	return acquirer.acquirer.AcquireGitHubVerified(ctx, repository, revision, subpath, digest)
}

func newDirectoryClient(dataRoot string) (*directoryv1.Client, error) {
	publicKey, err := base64.StdEncoding.Strict().DecodeString(defaultDirectoryPublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("decode Directory public key")
	}
	trust := directoryv1.TrustStore{Keys: []directoryv1.TrustedKey{{ID: defaultDirectoryKeyID, PublicKey: ed25519.PublicKey(publicKey), State: directoryv1.KeyCurrent}}}
	embedded, _, err := directoryv1.DecodeReleaseBootstrap(generatedProductionDirectoryBootstrap, trust)
	if err != nil {
		return nil, fmt.Errorf("load generated production Directory bootstrap: %w", err)
	}
	origin, err := productionDirectoryOrigin(os.Getenv("AGENTPLUGINS_DIRECTORY_ORIGIN"))
	if err != nil {
		return nil, err
	}
	return &directoryv1.Client{
		Origin: origin, HTTPClient: hardenedHTTPClient(),
		Trust: trust,
		// The production bootstrap is intentionally absent until the first
		// post-merge Directory publication. Short-name resolution fails closed
		// until a subsequent release binds that exact signed publication here.
		Embedded: embedded, RequireEmbeddedBootstrap: true,
		Cache: directoryv1.Cache{Path: filepath.Join(dataRoot, "directory-v1-cache.json")},
	}, nil
}

func productionDirectoryOrigin(environmentValue string) (string, error) {
	origin := strings.TrimSpace(environmentValue)
	if origin == "" {
		origin = defaultDirectoryOrigin
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != "https" || parsed.Opaque != "" || parsed.Host == "" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", fmt.Errorf("AGENTPLUGINS_DIRECTORY_ORIGIN must be an absolute credential-free HTTPS URL without query or fragment")
	}
	if !strings.HasSuffix(parsed.Path, "/") || parsed.RawPath != "" || strings.Contains(parsed.Path, "\\") || path.Clean(parsed.Path) != strings.TrimSuffix(parsed.Path, "/") {
		return "", fmt.Errorf("AGENTPLUGINS_DIRECTORY_ORIGIN must have a clean, unescaped directory path ending in /")
	}
	return parsed.String(), nil
}

func agentpluginsHome() (string, error) {
	if explicit := strings.TrimSpace(os.Getenv("AGENTPLUGINS_HOME")); explicit != "" {
		absolute, err := filepath.Abs(explicit)
		if err != nil {
			return "", fmt.Errorf("resolve AGENTPLUGINS_HOME: %w", err)
		}
		return filepath.Clean(absolute), nil
	}
	root, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(root, "agentplugins"), nil
}

func hardenedHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) > 2 {
				return fmt.Errorf("too many Directory redirects")
			}
			if request.URL.Scheme != "https" || len(via) == 0 ||
				!strings.EqualFold(request.URL.Scheme, via[0].URL.Scheme) ||
				!strings.EqualFold(request.URL.Host, via[0].URL.Host) {
				return fmt.Errorf("Directory redirect must remain on the original HTTPS origin")
			}
			request.Header.Del("Authorization")
			request.Header.Del("Cookie")
			request.Header.Del("Proxy-Authorization")
			return nil
		},
	}
}

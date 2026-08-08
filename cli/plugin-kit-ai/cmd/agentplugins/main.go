package main

import (
	"context"
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/777genius/plugin-kit-ai/cli/internal/agentpluginscli"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/dirswap"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/locks"
	processadapter "github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/process"
	sourceadapter "github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/source"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/clientdetect"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/loader"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/processlock"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/specregistry"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/statemigration"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/statev2"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/providers"
	"golang.org/x/term"
)

var (
	version              = "0.1.0-beta.1-development"
	defaultCatalogURL    = ""
	defaultCatalogDigest = "sha256:207df0cd3932d305bbc265357d1a7f6b68ef314ff725629db6ebe27d4c403915"
	//go:embed catalog-v1.json
	embeddedCatalog []byte
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
	registry, err := specregistry.New()
	if err != nil {
		return err
	}
	runner := processadapter.OS{}
	catalogURL := firstNonEmpty(os.Getenv("AGENTPLUGINS_CATALOG_URL"), defaultCatalogURL)
	catalogDigest := firstNonEmpty(os.Getenv("AGENTPLUGINS_CATALOG_DIGEST"), defaultCatalogDigest)
	catalogBody := embeddedCatalog
	if strings.TrimSpace(os.Getenv("AGENTPLUGINS_CATALOG_URL")) != "" {
		catalogBody = nil
	}
	v2Store := statev2.Store{Path: filepath.Join(dataRoot, "state-v2.json")}
	legacyStatePath := filepath.Join(home, ".plugin-kit-ai", "state.json")
	migrator := statemigration.Migrator{
		LegacyPath: legacyStatePath,
		V2Store:    v2Store,
	}
	app := agentpluginscli.App{
		Version:         version,
		UserHome:        home,
		ManagedRoot:     filepath.Join(dataRoot, "managed"),
		StateStore:      v2Store,
		StateMigrator:   &migrator,
		LegacyLifecycle: agentpluginscli.NewLegacyLifecycle(legacyStatePath),
		LegacyStateLock: locks.FileLock{BaseDir: filepath.Join(home, ".plugin-kit-ai", "locks")},
		Directory:       dirswap.Manager{JournalDir: filepath.Join(dataRoot, "operations-v2")},
		Detector:        clientdetect.NewOS(home),
		SourceResolver:  sourceadapter.Resolver{Runner: runner, DisableAliases: true},
		PackageLoader:   loader.Loader{Registry: registry},
		Stager:          providers.Stager{},
		Activator:       providers.Activator{},
		MutationLock:    processlock.Lock{Path: filepath.Join(dataRoot, "mutation.lock")},
		HTTPClient:      hardenedHTTPClient(),
		CatalogURL:      catalogURL,
		CatalogDigest:   catalogDigest,
		CatalogBody:     catalogBody,
		Input:           os.Stdin,
		Output:          os.Stdout,
		ErrorOutput:     os.Stderr,
		Terminal:        term.IsTerminal(int(os.Stdin.Fd())),
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return agentpluginscli.NewRoot(app).ExecuteContext(ctx)
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
			if len(via) >= 5 {
				return fmt.Errorf("too many catalog redirects")
			}
			if request.URL.Scheme != "https" {
				return fmt.Errorf("catalog redirect must remain HTTPS")
			}
			request.Header.Del("Authorization")
			return nil
		},
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

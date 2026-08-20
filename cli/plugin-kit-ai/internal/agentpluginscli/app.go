package agentpluginscli

import (
	"io"
	"net/http"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/dirswap"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/statemigration"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/ports"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/transaction"
	legacyports "github.com/777genius/plugin-kit-ai/install/integrationctl/ports"
)

type App struct {
	Version         string
	UserHome        string
	ManagedRoot     string
	StateStore      transaction.StateStore
	Directory       dirswap.Manager
	Detector        ports.ClientDetector
	SourceResolver  legacyports.SourceResolver
	PackageLoader   ports.PackageLoader
	Stager          ports.PackageStager
	Activator       ports.ClientActivator
	MutationLock    ports.MutationLock
	StateMigrator   *statemigration.Migrator
	LegacyLifecycle ports.LegacyLifecycle
	LegacyStateLock legacyports.LockManager
	SourceSwitcher  SourceSwitcher
	DataPurger      DataPurger
	GroupLifecycle  GroupLifecycle
	HTTPClient      *http.Client
	CatalogURL      string
	CatalogDigest   string
	CatalogBody     []byte
	Input           io.Reader
	Output          io.Writer
	ErrorOutput     io.Writer
	Terminal        bool
}

type options struct {
	target              string
	scope               string
	dryRun              bool
	format              string
	noColor             bool
	externalUninstalled bool
	purgeData           bool
}

func (app App) input() io.Reader {
	if app.Input != nil {
		return app.Input
	}
	return strings.NewReader("")
}

func (app App) output() io.Writer {
	if app.Output != nil {
		return app.Output
	}
	return io.Discard
}

func (app App) errorOutput() io.Writer {
	if app.ErrorOutput != nil {
		return app.ErrorOutput
	}
	return io.Discard
}

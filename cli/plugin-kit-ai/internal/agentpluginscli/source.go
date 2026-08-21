package agentpluginscli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/adapters/catalog"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	legacydomain "github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
)

const maxCatalogBytes = 4 << 20

var shortNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,63}$`)

type loadedPackage struct {
	envelope domain.PackageEnvelope
	hints    domain.CompatibilityHints
	cleanup  func() error
}

func (app App) loadPackage(ctx context.Context, raw string) (loadedPackage, error) {
	if app.SourceResolver == nil || app.PackageLoader == nil || app.NativePackageLoader == nil {
		return loadedPackage{}, fmt.Errorf("package source dependencies are unavailable")
	}
	requested := strings.TrimSpace(raw)
	if requested == "" {
		return loadedPackage{}, fmt.Errorf("plugin name or source is required")
	}
	resolvedRef := requested
	var catalogResolution *domain.CatalogResolution
	if isShortName(requested) && !localDirectoryExists(requested) {
		resolution, err := app.resolveCatalogName(ctx, requested)
		if err != nil {
			return loadedPackage{}, err
		}
		catalogResolution = &resolution
		resolvedRef = resolution.SourceReference
	}
	resolved, err := app.SourceResolver.Resolve(ctx, legacydomain.IntegrationRef{Raw: resolvedRef})
	if err != nil {
		return loadedPackage{}, err
	}
	cleanup := resolved.Cleanup
	fail := func(cause error) (loadedPackage, error) {
		if cleanup != nil {
			_ = cleanup()
		}
		return loadedPackage{}, cause
	}
	sourceIdentity := domain.SourceIdentity{
		RequestedSource:  requested,
		CanonicalSource:  firstNonBlank(resolved.CanonicalSource, resolved.Resolved.Value),
		Repository:       resolved.Repository,
		PackageSubpath:   resolved.PackageSubpath,
		ResolvedRevision: firstNonBlank(resolved.ResolvedRevision, revisionFromResolved(resolved.Resolved.Value)),
	}
	if resolved.Kind == "local_path" {
		sourceIdentity.CanonicalSource = resolved.Resolved.Value
		sourceIdentity.ResolvedRevision = ""
	}
	hints := domain.CompatibilityHints{}
	if catalogResolution != nil {
		sourceIdentity.Repository = catalogResolution.SourceReference[:strings.Index(catalogResolution.SourceReference, "@")]
		sourceIdentity.PackageSubpath = catalogResolution.Entry.SourcePath
		sourceIdentity.ResolvedRevision = revisionFromCatalogReference(catalogResolution.SourceReference)
		sourceIdentity.CanonicalSource = "https://github.com/" + sourceIdentity.Repository + "//" + sourceIdentity.PackageSubpath
		hints = catalogResolution.Hints
	}
	envelope, err := app.loadAcquiredSnapshot(ctx, domain.LoadInput{
		SnapshotRoot: resolved.LocalPath, TreeDigest: resolved.SourceDigest,
		ExecutableFiles: resolved.ExecutableFiles, Source: sourceIdentity,
	})
	if err != nil {
		return fail(err)
	}
	if catalogResolution != nil {
		if envelope.Manifest.Name != catalogResolution.Entry.Name || envelope.Manifest.Version != catalogResolution.Entry.Version {
			return fail(fmt.Errorf("catalog identity does not match resolved package manifest"))
		}
		if envelope.TreeDigest != catalogResolution.Entry.TreeDigest || envelope.ManifestDigest != catalogResolution.Entry.ManifestDigest {
			return fail(fmt.Errorf("catalog package digest mismatch"))
		}
		evidence := catalogResolution.Evidence
		evidence.Compatibility = cloneCatalogCompatibility(evidence.Compatibility)
		envelope.CatalogEvidence = &evidence
	}
	return loadedPackage{envelope: envelope, hints: hints, cleanup: cleanup}, nil
}

// loadAcquiredSnapshot selects a manifest format from the immutable snapshot.
// A root plugin.json has absolute precedence: even a directory, symlink, or
// malformed file is sent to the portable loader so its exact error is kept.
func (app App) loadAcquiredSnapshot(ctx context.Context, input domain.LoadInput) (domain.PackageEnvelope, error) {
	if _, err := os.Lstat(filepath.Join(input.SnapshotRoot, "plugin.json")); err == nil || !os.IsNotExist(err) {
		return app.PackageLoader.Load(ctx, input)
	}
	if _, err := os.Lstat(filepath.Join(input.SnapshotRoot, ".codex-plugin", "plugin.json")); err == nil || !os.IsNotExist(err) {
		return app.NativePackageLoader.Load(ctx, input)
	}
	// The compatibility importer owns the established diagnostic that names
	// both accepted manifest locations when neither one exists.
	return app.NativePackageLoader.Load(ctx, input)
}

func cloneCatalogCompatibility(source map[string]domain.CatalogCompatibility) map[string]domain.CatalogCompatibility {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]domain.CatalogCompatibility, len(source))
	for client, compatibility := range source {
		if compatibility.AppBinding != nil {
			binding := *compatibility.AppBinding
			compatibility.AppBinding = &binding
		}
		result[client] = compatibility
	}
	return result
}

func prepareLoadedPackageForClient(loaded *loadedPackage, clientID domain.ClientID) error {
	if loaded == nil || clientID != domain.ClientChatGPT {
		return nil
	}
	compatibility, ok := loaded.hints.Compatibility[string(domain.ClientChatGPT)]
	if !ok || compatibility.AppBinding == nil {
		return nil
	}
	binding := *compatibility.AppBinding
	if err := catalog.ValidateAppBinding(binding); err != nil {
		return fmt.Errorf("catalog ChatGPT app binding is invalid: %w", err)
	}
	server, ok := loaded.envelope.MCP.Servers[binding.MCPServer]
	if !ok || !loaded.envelope.MCP.Enabled {
		return fmt.Errorf("catalog ChatGPT app binding references missing MCP server %q", binding.MCPServer)
	}
	serverURL, ok := server.Decoded["url"].(string)
	if !ok || serverURL != binding.MCPURL {
		return fmt.Errorf("catalog ChatGPT app binding URL does not match MCP server %q", binding.MCPServer)
	}
	if loaded.envelope.App.Present {
		existing, matches := loaded.envelope.App.Bindings[binding.AppKey]
		if !loaded.envelope.App.Enabled || !matches || len(loaded.envelope.App.Bindings) != 1 || existing.ID != binding.ID {
			return fmt.Errorf("package .app.json does not exactly match the catalog ChatGPT app binding")
		}
		return nil
	}
	entry := struct {
		ID string `json:"id"`
	}{ID: binding.ID}
	document := struct {
		Apps map[string]any `json:"apps"`
	}{Apps: map[string]any{binding.AppKey: entry}}
	raw, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode catalog ChatGPT app binding: %w", err)
	}
	entryRaw, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode catalog ChatGPT app entry: %w", err)
	}
	loaded.envelope.App = domain.AppComponent{
		Present: true, Declared: true, Enabled: true, Raw: raw,
		Bindings: map[string]domain.AppBinding{
			binding.AppKey: {Alias: binding.AppKey, ID: binding.ID, Raw: entryRaw},
		},
	}
	loaded.envelope.Inventory.AppPresent = true
	loaded.envelope.Inventory.AppBindings = []string{binding.AppKey}
	return nil
}

func restoreCatalogEvidence(loaded *loadedPackage, binding domain.ClientBinding) {
	if loaded == nil || loaded.envelope.CatalogEvidence != nil || binding.PackageRevision == nil || binding.PackageRevision.CatalogEvidence == nil {
		return
	}
	evidence := *binding.PackageRevision.CatalogEvidence
	evidence.Compatibility = cloneCatalogCompatibility(evidence.Compatibility)
	loaded.envelope.CatalogEvidence = &evidence
	loaded.hints.Compatibility = cloneCatalogCompatibility(evidence.Compatibility)
}

func (app App) resolveCatalogName(ctx context.Context, name string) (domain.CatalogResolution, error) {
	body := append([]byte(nil), app.CatalogBody...)
	if len(body) == 0 {
		if strings.TrimSpace(app.CatalogURL) == "" || strings.TrimSpace(app.CatalogDigest) == "" {
			return domain.CatalogResolution{}, fmt.Errorf("short-name catalog is not pinned in this build; use an exact source path")
		}
		var err error
		body, err = app.fetchCatalog(ctx)
		if err != nil {
			return domain.CatalogResolution{}, err
		}
	}
	loaded, err := (catalog.Loader{CurrentCLIVersion: app.Version}).Load(body, app.CatalogDigest)
	if err != nil {
		return domain.CatalogResolution{}, err
	}
	return loaded.Resolve(name)
}

func (app App) fetchCatalog(ctx context.Context) ([]byte, error) {
	parsed, err := url.Parse(app.CatalogURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, fmt.Errorf("catalog URL must be an absolute HTTPS URL")
	}
	client := app.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download pinned catalog: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return nil, fmt.Errorf("download pinned catalog: HTTP %d", response.StatusCode)
	}
	reader := io.LimitReader(response.Body, maxCatalogBytes+1)
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read pinned catalog: %w", err)
	}
	if len(body) > maxCatalogBytes {
		return nil, fmt.Errorf("pinned catalog exceeds %d bytes", maxCatalogBytes)
	}
	return body, nil
}

func isShortName(value string) bool {
	return shortNamePattern.MatchString(value) && !strings.Contains(value, string(filepath.Separator))
}

func localDirectoryExists(value string) bool {
	info, err := os.Stat(value)
	return err == nil && info.IsDir()
}

func revisionFromResolved(value string) string {
	index := strings.LastIndex(value, "@")
	if index < 0 {
		return ""
	}
	return strings.TrimSpace(value[index+1:])
}

func revisionFromCatalogReference(value string) string {
	at := strings.Index(value, "@")
	subpath := strings.Index(value, "//")
	if at < 0 || subpath < 0 || subpath <= at {
		return ""
	}
	return value[at+1 : subpath]
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

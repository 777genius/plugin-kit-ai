package generator

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/777genius/plugin-kit-ai/sdk/internal/descriptors/defs"
	"github.com/777genius/plugin-kit-ai/sdk/internal/runtime"
	"github.com/777genius/plugin-kit-ai/sdk/platformmeta"
)

type Artifact struct {
	Path    string
	Content []byte
}

type model struct {
	profiles map[runtime.PlatformID]defs.PlatformProfile
	cliProfiles []platformmeta.PlatformProfile
	events   []defs.EventDescriptor
}

func RenderArtifacts() ([]Artifact, error) {
	m, err := loadModel()
	if err != nil {
		return nil, err
	}
	var artifacts []Artifact
	artifacts = append(artifacts,
		Artifact{Path: "sdk/plugin-kit-ai/internal/descriptors/gen/registry_gen.go", Content: mustGo(renderRegistry(m))},
		Artifact{Path: "sdk/plugin-kit-ai/internal/descriptors/gen/resolvers_gen.go", Content: mustGo(renderResolvers(m))},
		Artifact{Path: "sdk/plugin-kit-ai/internal/descriptors/gen/support_gen.go", Content: mustGo(renderSupport(m))},
		Artifact{Path: "sdk/plugin-kit-ai/internal/descriptors/gen/completeness_gen_test.go", Content: mustGo(renderCompletenessTest(m))},
		Artifact{Path: "cli/plugin-kit-ai/internal/scaffold/platforms_gen.go", Content: mustGo(renderScaffoldPlatforms(m))},
		Artifact{Path: "cli/plugin-kit-ai/internal/validate/rules_gen.go", Content: mustGo(renderValidateRules(m))},
		Artifact{Path: "docs/generated/support_matrix.md", Content: []byte(renderSupportMatrix(m))},
	)
	for _, p := range runtimeProfiles(m) {
		artifacts = append(artifacts, Artifact{
			Path:    fmt.Sprintf("sdk/plugin-kit-ai/%s/registrar_gen.go", p.PublicPackage),
			Content: mustGo(renderRegistrar(m, p)),
		})
	}
	return artifacts, nil
}

func WriteAll(repoRoot string) error {
	arts, err := RenderArtifacts()
	if err != nil {
		return err
	}
	for _, art := range arts {
		full := filepath.Join(repoRoot, art.Path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, art.Content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func loadModel() (model, error) {
	profiles := defs.Profiles()
	events := defs.Events()
	out := model{
		profiles:   make(map[runtime.PlatformID]defs.PlatformProfile, len(profiles)),
		cliProfiles: append([]platformmeta.PlatformProfile(nil), platformmeta.All()...),
		events:     events,
	}
	for _, p := range profiles {
		if p.Platform == "" {
			return model{}, fmt.Errorf("platform profile missing platform")
		}
		if _, ok := out.profiles[p.Platform]; ok {
			return model{}, fmt.Errorf("duplicate platform profile %s", p.Platform)
		}
		out.profiles[p.Platform] = p
	}
	if err := validateModel(out); err != nil {
		return model{}, err
	}
	return out, nil
}

func validateModel(m model) error {
	seenEvents := make(map[string]struct{}, len(m.events))
	for _, p := range m.profiles {
		if p.Status == "" {
			return fmt.Errorf("platform profile %s missing status", p.Platform)
		}
		if p.PublicPackage == "" || p.InternalPackage == "" || p.InternalImport == "" {
			return fmt.Errorf("platform profile %s missing package metadata", p.Platform)
		}
		if len(p.TransportModes) == 0 {
			return fmt.Errorf("platform profile %s missing transport modes", p.Platform)
		}
		if p.Status != runtime.StatusDeferred {
			if len(p.Scaffold.RequiredFiles) == 0 || len(p.Scaffold.TemplateFiles) == 0 {
				return fmt.Errorf("platform profile %s missing scaffold metadata", p.Platform)
			}
			if len(p.Validate.RequiredFiles) == 0 {
				return fmt.Errorf("platform profile %s missing validate metadata", p.Platform)
			}
		}
	}

	registrars := make(map[string]struct{})
	for _, e := range m.events {
		k := string(e.Platform) + "/" + string(e.Event)
		if _, ok := seenEvents[k]; ok {
			return fmt.Errorf("duplicate event descriptor %s", k)
		}
		seenEvents[k] = struct{}{}
		p, ok := m.profiles[e.Platform]
		if !ok {
			return fmt.Errorf("event descriptor %s references unknown platform profile", k)
		}
		if p.Status != runtime.StatusRuntimeSupported {
			return fmt.Errorf("event descriptor %s targets non-runtime profile %s", k, p.Status)
		}
		if e.Invocation.Kind == "" {
			return fmt.Errorf("event descriptor %s missing invocation kind", k)
		}
		if e.Invocation.Kind != runtime.InvocationCustomResolver && strings.TrimSpace(e.Invocation.Name) == "" {
			return fmt.Errorf("event descriptor %s missing invocation name", k)
		}
		if e.Contract.Maturity == "" {
			return fmt.Errorf("event descriptor %s missing contract maturity", k)
		}
		if e.DecodeFunc == "" || e.EncodeFunc == "" {
			return fmt.Errorf("event descriptor %s missing codec refs", k)
		}
		if e.Registrar.MethodName == "" || e.Registrar.WrapFunc == "" {
			return fmt.Errorf("event descriptor %s missing registrar metadata", k)
		}
		if e.Docs.Summary == "" || e.Docs.SnippetKey == "" || e.Docs.TableGroup == "" {
			return fmt.Errorf("event descriptor %s missing docs metadata", k)
		}
		regKey := p.PublicPackage + "." + e.Registrar.MethodName
		if _, ok := registrars[regKey]; ok {
			return fmt.Errorf("registrar collision %s", regKey)
		}
		registrars[regKey] = struct{}{}
		switch e.Carrier {
		case runtime.CarrierStdinJSON, runtime.CarrierArgvJSON:
		default:
			return fmt.Errorf("event descriptor %s has unsupported carrier %q", k, e.Carrier)
		}
		switch e.Invocation.Kind {
		case runtime.InvocationArgvCommand, runtime.InvocationArgvCommandCaseFold:
		case runtime.InvocationCustomResolver:
			if strings.TrimSpace(e.Invocation.ResolverRef) == "" {
				return fmt.Errorf("event descriptor %s missing custom resolver ref", k)
			}
		default:
			return fmt.Errorf("event descriptor %s has unsupported invocation kind %q", k, e.Invocation.Kind)
		}
	}
	return nil
}

func mustGo(src string) []byte {
	b, err := format.Source([]byte(src))
	if err != nil {
		panic(err)
	}
	return b
}

func renderRegistry(m model) string {
	var b strings.Builder
	b.WriteString("package gen\n\n")
	b.WriteString("import (\n")
	for _, p := range runtimeProfiles(m) {
		b.WriteString(fmt.Sprintf("\t%s %q\n", internalAlias(p.Platform), p.InternalImport))
	}
	b.WriteString("\t\"github.com/777genius/plugin-kit-ai/sdk/internal/runtime\"\n")
	b.WriteString(")\n\n")
	b.WriteString("type key struct { platform runtime.PlatformID; event runtime.EventID }\n\n")
	b.WriteString("var registry = map[key]runtime.Descriptor{\n")
	for _, e := range m.events {
		p := m.profiles[e.Platform]
		b.WriteString(fmt.Sprintf("\t{platform: %q, event: %q}: {\n", e.Platform, e.Event))
		b.WriteString(fmt.Sprintf("\t\tPlatform: %q,\n", e.Platform))
		b.WriteString(fmt.Sprintf("\t\tEvent: %q,\n", e.Event))
		b.WriteString(fmt.Sprintf("\t\tCarrier: %s,\n", carrierExpr(e.Carrier)))
		b.WriteString(fmt.Sprintf("\t\tDecode: %s.%s,\n", internalAlias(p.Platform), e.DecodeFunc))
		b.WriteString(fmt.Sprintf("\t\tEncode: %s.%s,\n", internalAlias(p.Platform), e.EncodeFunc))
		b.WriteString("\t},\n")
	}
	b.WriteString("}\n\n")
	b.WriteString("func Lookup(platform runtime.PlatformID, event runtime.EventID) (runtime.Descriptor, bool) {\n")
	b.WriteString("\td, ok := registry[key{platform: platform, event: event}]\n")
	b.WriteString("\treturn d, ok\n")
	b.WriteString("}\n")
	return b.String()
}

func renderResolvers(m model) string {
	var b strings.Builder
	b.WriteString("package gen\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"fmt\"\n")
	b.WriteString("\t\"strings\"\n")
	b.WriteString("\t\"github.com/777genius/plugin-kit-ai/sdk/internal/runtime\"\n")
	b.WriteString(")\n\n")
	b.WriteString("func ResolveInvocation(args []string, _ runtime.Env) (runtime.Invocation, error) {\n")
	b.WriteString("\tif len(args) < 2 {\n")
	b.WriteString("\t\treturn runtime.Invocation{}, fmt.Errorf(\"usage: <binary> <hookName>\")\n")
	b.WriteString("\t}\n")
	b.WriteString("\traw := args[1]\n")
	for _, e := range m.events {
		switch e.Invocation.Kind {
		case runtime.InvocationArgvCommand:
			b.WriteString(fmt.Sprintf("\tif raw == %q {\n", e.Invocation.Name))
		case runtime.InvocationArgvCommandCaseFold:
			b.WriteString(fmt.Sprintf("\tif strings.EqualFold(raw, %q) {\n", e.Invocation.Name))
		case runtime.InvocationCustomResolver:
			continue
		}
		b.WriteString(fmt.Sprintf("\t\treturn runtime.Invocation{Platform: %q, Event: %q, RawName: raw}, nil\n", e.Platform, e.Event))
		b.WriteString("\t}\n")
	}
	b.WriteString("\treturn runtime.Invocation{}, fmt.Errorf(\"unknown invocation %q\", raw)\n")
	b.WriteString("}\n")
	return b.String()
}

func renderSupport(m model) string {
	var b strings.Builder
	b.WriteString("package gen\n\n")
	b.WriteString("import \"github.com/777genius/plugin-kit-ai/sdk/internal/runtime\"\n\n")
	b.WriteString("func AllSupportEntries() []runtime.SupportEntry {\n")
	b.WriteString("\treturn []runtime.SupportEntry{\n")
	for _, e := range m.events {
		p := m.profiles[e.Platform]
		b.WriteString("\t\t{\n")
		b.WriteString(fmt.Sprintf("\t\t\tPlatform: %q,\n", e.Platform))
		b.WriteString(fmt.Sprintf("\t\t\tEvent: %q,\n", e.Event))
		b.WriteString(fmt.Sprintf("\t\t\tStatus: %q,\n", p.Status))
		b.WriteString(fmt.Sprintf("\t\t\tMaturity: %q,\n", e.Contract.Maturity))
		b.WriteString(fmt.Sprintf("\t\t\tV1Target: %t,\n", e.Contract.V1Target))
		b.WriteString(fmt.Sprintf("\t\t\tInvocationKind: %q,\n", e.Invocation.Kind))
		b.WriteString(fmt.Sprintf("\t\t\tCarrier: %s,\n", carrierExpr(e.Carrier)))
		b.WriteString("\t\t\tTransportModes: []runtime.TransportMode{\n")
		for _, mode := range p.TransportModes {
			b.WriteString(fmt.Sprintf("\t\t\t\t%q,\n", mode))
		}
		b.WriteString("\t\t\t},\n")
		b.WriteString(fmt.Sprintf("\t\t\tScaffoldSupport: %t,\n", len(p.Scaffold.RequiredFiles) > 0))
		b.WriteString(fmt.Sprintf("\t\t\tValidateSupport: %t,\n", len(p.Validate.RequiredFiles) > 0))
		b.WriteString("\t\t\tCapabilities: []runtime.CapabilityID{\n")
		for _, cap := range e.Capabilities {
			b.WriteString(fmt.Sprintf("\t\t\t\t%q,\n", cap))
		}
		b.WriteString("\t\t\t},\n")
		b.WriteString(fmt.Sprintf("\t\t\tSummary: %q,\n", e.Docs.Summary))
		b.WriteString(fmt.Sprintf("\t\t\tLiveTestProfile: %q,\n", p.LiveTestProfile))
		b.WriteString("\t\t},\n")
	}
	b.WriteString("\t}\n")
	b.WriteString("}\n")
	return b.String()
}

func renderCompletenessTest(m model) string {
	var b strings.Builder
	b.WriteString("package gen\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"testing\"\n")
	b.WriteString("\t\"github.com/777genius/plugin-kit-ai/sdk/internal/descriptors/defs\"\n")
	b.WriteString(")\n\n")
	b.WriteString("func TestGeneratedRegistryCompleteness(t *testing.T) {\n")
	b.WriteString("\tprofiles := defs.Profiles()\n")
	b.WriteString("\tevents := defs.Events()\n")
	b.WriteString(fmt.Sprintf("\tif len(profiles) != %d { t.Fatalf(\"profiles count = %%d\", len(profiles)) }\n", len(m.profiles)))
	b.WriteString(fmt.Sprintf("\tif len(events) != %d { t.Fatalf(\"events count = %%d\", len(events)) }\n", len(m.events)))
	b.WriteString("\tentries := AllSupportEntries()\n")
	b.WriteString("\tif len(entries) != len(events) { t.Fatalf(\"support entries = %d want %d\", len(entries), len(events)) }\n")
	b.WriteString("\tfor _, event := range events {\n")
	b.WriteString("\t\tif _, ok := Lookup(event.Platform, event.Event); !ok { t.Fatalf(\"missing descriptor %s/%s\", event.Platform, event.Event) }\n")
	b.WriteString("\t\tif event.Invocation.Kind == \"\" { t.Fatalf(\"missing invocation kind for %s/%s\", event.Platform, event.Event) }\n")
	b.WriteString("\t\tif event.Contract.Maturity == \"\" { t.Fatalf(\"missing contract maturity for %s/%s\", event.Platform, event.Event) }\n")
	b.WriteString("\t\tif event.DecodeFunc == \"\" || event.EncodeFunc == \"\" { t.Fatalf(\"missing codec refs for %s/%s\", event.Platform, event.Event) }\n")
	b.WriteString("\t\tif event.Registrar.MethodName == \"\" || event.Registrar.WrapFunc == \"\" { t.Fatalf(\"missing registrar metadata for %s/%s\", event.Platform, event.Event) }\n")
	b.WriteString("\t}\n")
	b.WriteString("\tfor _, profile := range profiles {\n")
	b.WriteString("\t\tif profile.Status == \"\" { t.Fatalf(\"missing status for %s\", profile.Platform) }\n")
	b.WriteString("\t\tif len(profile.TransportModes) == 0 { t.Fatalf(\"missing transport modes for %s\", profile.Platform) }\n")
	b.WriteString("\t\tif profile.Status != \"deferred\" {\n")
	b.WriteString("\t\t\tif len(profile.Scaffold.RequiredFiles) == 0 || len(profile.Scaffold.TemplateFiles) == 0 { t.Fatalf(\"missing scaffold metadata for %s\", profile.Platform) }\n")
	b.WriteString("\t\t\tif len(profile.Validate.RequiredFiles) == 0 { t.Fatalf(\"missing validate metadata for %s\", profile.Platform) }\n")
	b.WriteString("\t\t}\n")
	b.WriteString("\t}\n")
	b.WriteString("}\n")
	return b.String()
}

func renderRegistrar(m model, p defs.PlatformProfile) string {
	var b strings.Builder
	b.WriteString("package " + p.PublicPackage + "\n\n")
	b.WriteString("func generatedRegistrarMarker() {}\n\n")
	for _, e := range eventsForPlatform(m, p.Platform) {
		b.WriteString(fmt.Sprintf("func (r *Registrar) %s(fn func(%s) %s) {\n", e.Registrar.MethodName, e.Registrar.EventType, e.Registrar.ResponseType))
		b.WriteString(fmt.Sprintf("\tr.backend.Register(%q, %q, %s(fn))\n", e.Platform, e.Event, e.Registrar.WrapFunc))
		b.WriteString("}\n\n")
	}
	return b.String()
}

func renderScaffoldPlatforms(m model) string {
	var b strings.Builder
	b.WriteString("package scaffold\n\n")
	b.WriteString("import (\n\t\"strings\"\n)\n\n")
	b.WriteString("var generatedPlatforms = map[string]PlatformDefinition{\n")
	for _, p := range scaffoldTargetProfiles(m) {
		b.WriteString(fmt.Sprintf("\t%q: {\n", p.ID))
		b.WriteString(fmt.Sprintf("\t\tName: %q,\n", p.ID))
		b.WriteString("\t\tFiles: []TemplateFile{\n")
		for _, file := range p.Scaffold.TemplateFiles {
			b.WriteString(fmt.Sprintf("\t\t\t{Path: %q, Template: %q, Extra: %t},\n", file.Path, file.Template, file.Extra))
		}
		b.WriteString("\t\t},\n")
		b.WriteString("\t},\n")
	}
	b.WriteString("}\n\n")
	b.WriteString("func LookupPlatform(name string) (PlatformDefinition, bool) {\n")
	b.WriteString("\tp, ok := generatedPlatforms[normalizePlatform(name)]\n\treturn p, ok\n}\n\n")
	b.WriteString("func normalizePlatform(name string) string {\n")
	b.WriteString("\tname = strings.ToLower(strings.TrimSpace(name))\n")
	b.WriteString("\tif name == \"\" { return \"codex-runtime\" }\n")
	b.WriteString("\treturn name\n")
	b.WriteString("}\n")
	return b.String()
}

func renderValidateRules(m model) string {
	var b strings.Builder
	b.WriteString("package validate\n\n")
	b.WriteString("import \"strings\"\n\n")
	b.WriteString("var generatedRules = map[string]Rule{\n")
	for _, p := range scaffoldTargetProfiles(m) {
		b.WriteString(fmt.Sprintf("\t%q: {\n", p.ID))
		b.WriteString(fmt.Sprintf("\t\tPlatform: %q,\n", p.ID))
		b.WriteString("\t\tRequiredFiles: []string{\n")
		for _, s := range p.Validate.RequiredFiles {
			b.WriteString(fmt.Sprintf("\t\t\t%q,\n", s))
		}
		b.WriteString("\t\t},\n")
		b.WriteString("\t\tForbiddenFiles: []string{\n")
		for _, s := range p.Validate.ForbiddenFiles {
			b.WriteString(fmt.Sprintf("\t\t\t%q,\n", s))
		}
		b.WriteString("\t\t},\n")
		b.WriteString("\t\tBuildTargets: []string{\n")
		for _, s := range p.Validate.BuildTargets {
			b.WriteString(fmt.Sprintf("\t\t\t%q,\n", s))
		}
		b.WriteString("\t\t},\n")
		b.WriteString("\t},\n")
	}
	b.WriteString("}\n\n")
	b.WriteString("func LookupRule(name string) (Rule, bool) {\n")
	b.WriteString("\tr, ok := generatedRules[normalizePlatform(name)]\n\treturn r, ok\n}\n\n")
	b.WriteString("func normalizePlatform(name string) string {\n")
	b.WriteString("\tname = strings.ToLower(strings.TrimSpace(name))\n")
	b.WriteString("\tif name == \"\" { return \"codex-runtime\" }\n")
	b.WriteString("\treturn name\n")
	b.WriteString("}\n")
	return b.String()
}

func renderSupportMatrix(m model) string {
	var b strings.Builder
	b.WriteString("# Generated Support Matrix\n\n")
	b.WriteString("This generated table is the canonical per-event runtime support contract for shipped runtime claims. Packaging-only targets such as Gemini are documented in SUPPORT.md and are intentionally not listed here.\n\n")
	b.WriteString("| Platform | Event | Status | Maturity | Contract Class | V1 Target | Invocation | Carrier | Transport Modes | Scaffold | Validate | Capabilities | Live Test | Summary |\n")
	b.WriteString("|----------|-------|--------|----------|----------------|-----------|------------|---------|-----------------|----------|----------|--------------|-----------|---------|\n")
	for _, e := range m.events {
		p := m.profiles[e.Platform]
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %t | %s | %s | %s | %t | %t | %s | %s | %s |\n",
			e.Platform,
			e.Event,
			p.Status,
			e.Contract.Maturity,
			contractClass(p, e),
			e.Contract.V1Target,
			e.Invocation.Kind,
			e.Carrier,
			joinTransportModes(p.TransportModes),
			len(p.Scaffold.RequiredFiles) > 0,
			len(p.Validate.RequiredFiles) > 0,
			joinCapabilities(e.Capabilities),
			p.LiveTestProfile,
			e.Docs.Summary,
		))
	}
	return b.String()
}

func contractClass(p defs.PlatformProfile, e defs.EventDescriptor) string {
	if p.Status == runtime.StatusRuntimeSupported {
		switch e.Contract.Maturity {
		case runtime.MaturityStable:
			return "production-ready"
		case runtime.MaturityExperimental:
			return "public-experimental"
		default:
			return "runtime-supported but not stable"
		}
	}
	switch e.Contract.Maturity {
	case runtime.MaturityExperimental:
		return "public-experimental"
	default:
		return "public-beta"
	}
}

func carrierExpr(c runtime.CarrierKind) string {
	switch c {
	case runtime.CarrierStdinJSON:
		return "runtime.CarrierStdinJSON"
	case runtime.CarrierArgvJSON:
		return "runtime.CarrierArgvJSON"
	default:
		panic("unsupported carrier")
	}
}

func internalAlias(platform runtime.PlatformID) string {
	return "internal_" + strings.ReplaceAll(string(platform), "-", "_")
}

func runtimeProfiles(m model) []defs.PlatformProfile {
	var out []defs.PlatformProfile
	for _, p := range sortedProfiles(m) {
		if p.Status == runtime.StatusRuntimeSupported {
			out = append(out, p)
		}
	}
	return out
}

func scaffoldProfiles(m model) []defs.PlatformProfile {
	var out []defs.PlatformProfile
	for _, p := range sortedProfiles(m) {
		if p.Status == runtime.StatusRuntimeSupported || p.Status == runtime.StatusScaffoldOnly {
			out = append(out, p)
		}
	}
	return out
}

func scaffoldTargetProfiles(m model) []platformmeta.PlatformProfile {
	var out []platformmeta.PlatformProfile
	for _, p := range m.cliProfiles {
		if p.SDK.Status == platformmeta.StatusRuntimeSupported || p.SDK.Status == platformmeta.StatusScaffoldOnly {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func sortedProfiles(m model) []defs.PlatformProfile {
	var out []defs.PlatformProfile
	for _, p := range m.profiles {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Platform < out[j].Platform
	})
	return out
}

func eventsForPlatform(m model, platform runtime.PlatformID) []defs.EventDescriptor {
	var out []defs.EventDescriptor
	for _, e := range m.events {
		if e.Platform == platform {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Event < out[j].Event
	})
	return out
}

func joinCapabilities(in []runtime.CapabilityID) string {
	out := make([]string, 0, len(in))
	for _, cap := range in {
		out = append(out, string(cap))
	}
	return strings.Join(out, ", ")
}

func joinTransportModes(in []runtime.TransportMode) string {
	out := make([]string, 0, len(in))
	for _, mode := range in {
		out = append(out, string(mode))
	}
	return strings.Join(out, ", ")
}

func FindRepoRoot(start string) (string, error) {
	dir := start
	for {
		mod := filepath.Join(dir, "go.mod")
		b, err := os.ReadFile(mod)
		if err == nil && bytes.HasPrefix(b, []byte("module github.com/plugin-kit-ai/plugin-kit-ai\n")) {
			return dir, nil
		}
		next := filepath.Dir(dir)
		if next == dir {
			return "", fmt.Errorf("repo root not found from %s", start)
		}
		dir = next
	}
}

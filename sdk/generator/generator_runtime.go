package generator

import (
	"fmt"
	"go/format"
	"sort"
	"strings"

	"github.com/777genius/plugin-kit-ai/sdk/internal/descriptors/defs"
	"github.com/777genius/plugin-kit-ai/sdk/internal/runtime"
)

func renderArtifacts(m model) []Artifact {
	artifacts := []Artifact{
		{Path: "sdk/internal/descriptors/gen/registry_gen.go", Content: mustGo(renderRegistry(m))},
		{Path: "sdk/internal/descriptors/gen/resolvers_gen.go", Content: mustGo(renderResolvers(m))},
		{Path: "sdk/internal/descriptors/gen/support_gen.go", Content: mustGo(renderSupport(m))},
		{Path: "sdk/internal/descriptors/gen/completeness_gen_test.go", Content: mustGo(renderCompletenessTest(m))},
		{Path: "cli/plugin-kit-ai/internal/scaffold/platforms_gen.go", Content: mustGo(renderScaffoldPlatforms(m))},
		{Path: "cli/plugin-kit-ai/internal/validate/rules_gen.go", Content: mustGo(renderValidateRules(m))},
		{Path: "docs/generated/support_matrix.md", Content: []byte(renderSupportMatrix(m))},
		{Path: "docs/generated/target_support_matrix.md", Content: []byte(renderTargetSupportMatrix(m))},
	}
	for _, p := range runtimeProfiles(m) {
		artifacts = append(artifacts, Artifact{
			Path:    fmt.Sprintf("sdk/%s/registrar_gen.go", p.PublicPackage),
			Content: mustGo(renderRegistrar(m, p)),
		})
	}
	return artifacts
}

func mustGo(src string) []byte {
	body, err := format.Source([]byte(src))
	if err != nil {
		panic(err)
	}
	return body
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
	for _, e := range eventsForPlatform(m, p.Platform) {
		b.WriteString(fmt.Sprintf("// %s registers a handler for the %s %s.\n", e.Registrar.MethodName, strings.TrimSpace(platformLabel(e.Platform)), strings.TrimSpace(eventLabel(e.Event))))
		b.WriteString(fmt.Sprintf("func (r *Registrar) %s(fn func(%s) %s) {\n", e.Registrar.MethodName, e.Registrar.EventType, e.Registrar.ResponseType))
		b.WriteString(fmt.Sprintf("\tr.backend.Register(%q, %q, %s(fn))\n", e.Platform, e.Event, e.Registrar.WrapFunc))
		b.WriteString("}\n\n")
	}
	return b.String()
}

func platformLabel(platform runtime.PlatformID) string {
	switch platform {
	case "claude":
		return "Claude"
	case "codex":
		return "Codex"
	default:
		return string(platform)
	}
}

func eventLabel(event runtime.EventID) string {
	return string(event)
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

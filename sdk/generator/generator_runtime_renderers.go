package generator

import (
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/sdk/internal/runtime"
)

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

func renderSupportIndex() string {
	var b strings.Builder
	b.WriteString("package gen\n\n")
	b.WriteString("import \"github.com/777genius/plugin-kit-ai/sdk/internal/runtime\"\n\n")
	b.WriteString("func AllSupportEntries() []runtime.SupportEntry {\n")
	b.WriteString("\tentries := make([]runtime.SupportEntry, 0, len(claudeSupportEntries())+len(geminiSupportEntries())+len(codexSupportEntries()))\n")
	b.WriteString("\tentries = append(entries, claudeSupportEntries()...)\n")
	b.WriteString("\tentries = append(entries, geminiSupportEntries()...)\n")
	b.WriteString("\tentries = append(entries, codexSupportEntries()...)\n")
	b.WriteString("\treturn entries\n")
	b.WriteString("}\n")
	return b.String()
}

func renderSupportBucket(m model, platform string) string {
	var b strings.Builder
	b.WriteString("package gen\n\n")
	b.WriteString("import \"github.com/777genius/plugin-kit-ai/sdk/internal/runtime\"\n\n")
	b.WriteString(fmt.Sprintf("func %s() []runtime.SupportEntry {\n", supportBucketFuncName(platform)))
	b.WriteString("\treturn []runtime.SupportEntry{\n")
	for _, e := range m.events {
		if string(e.Platform) != platform {
			continue
		}
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

func supportBucketFuncName(platform string) string {
	switch platform {
	case "claude":
		return "claudeSupportEntries"
	case "gemini":
		return "geminiSupportEntries"
	case "codex":
		return "codexSupportEntries"
	default:
		panic(fmt.Sprintf("unsupported support bucket %q", platform))
	}
}

func renderCompletenessTest(m model) string {
	var b strings.Builder
	b.WriteString("package gen\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"github.com/777genius/plugin-kit-ai/sdk/internal/descriptors/defs\"\n")
	b.WriteString("\t\"testing\"\n")
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
	b.WriteString("\n")
	b.WriteString("func TestSupportEntriesPreserveEventOrder(t *testing.T) {\n")
	b.WriteString("\tt.Parallel()\n\n")
	b.WriteString("\tentries := AllSupportEntries()\n")
	b.WriteString("\tevents := defs.Events()\n")
	b.WriteString("\tif len(entries) != len(events) { t.Fatalf(\"support entries = %d want %d\", len(entries), len(events)) }\n")
	b.WriteString("\tfor i, event := range events {\n")
	b.WriteString("\t\tentry := entries[i]\n")
	b.WriteString("\t\tif entry.Platform != event.Platform || entry.Event != event.Event {\n")
	b.WriteString("\t\t\tt.Fatalf(\"entry[%d] = %s/%s want %s/%s\", i, entry.Platform, entry.Event, event.Platform, event.Event)\n")
	b.WriteString("\t\t}\n")
	b.WriteString("\t}\n")
	b.WriteString("}\n")
	return b.String()
}

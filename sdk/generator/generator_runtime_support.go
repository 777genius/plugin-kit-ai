package generator

import (
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/sdk/internal/descriptors/defs"
	"github.com/777genius/plugin-kit-ai/sdk/internal/runtime"
)

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
	for _, e := range supportBucketEvents(m, platform) {
		b.WriteString(renderSupportBucketEntry(m.profiles[e.Platform], e))
	}
	b.WriteString("\t}\n")
	b.WriteString("}\n")
	return b.String()
}

func supportBucketEvents(m model, platform string) []defs.EventDescriptor {
	events := make([]defs.EventDescriptor, 0, len(m.events))
	for _, e := range m.events {
		if string(e.Platform) == platform {
			events = append(events, e)
		}
	}
	return events
}

func renderSupportBucketEntry(profile defs.PlatformProfile, e defs.EventDescriptor) string {
	var b strings.Builder
	b.WriteString("\t\t{\n")
	b.WriteString(fmt.Sprintf("\t\t\tPlatform: %q,\n", e.Platform))
	b.WriteString(fmt.Sprintf("\t\t\tEvent: %q,\n", e.Event))
	b.WriteString(fmt.Sprintf("\t\t\tStatus: %q,\n", profile.Status))
	b.WriteString(fmt.Sprintf("\t\t\tMaturity: %q,\n", e.Contract.Maturity))
	b.WriteString(fmt.Sprintf("\t\t\tV1Target: %t,\n", e.Contract.V1Target))
	b.WriteString(fmt.Sprintf("\t\t\tInvocationKind: %q,\n", e.Invocation.Kind))
	b.WriteString(fmt.Sprintf("\t\t\tCarrier: %s,\n", carrierExpr(e.Carrier)))
	b.WriteString(renderTransportModes(profile.TransportModes))
	b.WriteString(fmt.Sprintf("\t\t\tScaffoldSupport: %t,\n", len(profile.Scaffold.RequiredFiles) > 0))
	b.WriteString(fmt.Sprintf("\t\t\tValidateSupport: %t,\n", len(profile.Validate.RequiredFiles) > 0))
	b.WriteString(renderSupportCapabilities(e.Capabilities))
	b.WriteString(fmt.Sprintf("\t\t\tSummary: %q,\n", e.Docs.Summary))
	b.WriteString(fmt.Sprintf("\t\t\tLiveTestProfile: %q,\n", profile.LiveTestProfile))
	b.WriteString("\t\t},\n")
	return b.String()
}

func renderTransportModes(modes []runtime.TransportMode) string {
	var b strings.Builder
	b.WriteString("\t\t\tTransportModes: []runtime.TransportMode{\n")
	for _, mode := range modes {
		b.WriteString(fmt.Sprintf("\t\t\t\t%q,\n", mode))
	}
	b.WriteString("\t\t\t},\n")
	return b.String()
}

func renderSupportCapabilities(caps []runtime.CapabilityID) string {
	var b strings.Builder
	b.WriteString("\t\t\tCapabilities: []runtime.CapabilityID{\n")
	for _, cap := range caps {
		b.WriteString(fmt.Sprintf("\t\t\t\t%q,\n", cap))
	}
	b.WriteString("\t\t\t},\n")
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

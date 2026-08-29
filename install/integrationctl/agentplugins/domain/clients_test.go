package domain

import (
	"reflect"
	"testing"
)

func TestClientRegistryHasStableOrderAndSharedCopilotBackend(t *testing.T) {
	want := []ClientID{
		ClientCodex, ClientChatGPT, ClientCursor, ClientCopilot, ClientVSCode, ClientKiro,
		ClientClaude, ClientGemini, ClientOpenCode, ClientCline, ClientWindsurf,
	}
	if got := SupportedClientIDs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("supported clients = %v, want %v", got, want)
	}
	if !SameClientBackend(ClientCopilot, ClientVSCode) || SameClientBackend(ClientCursor, ClientVSCode) {
		t.Fatal("Copilot / VS Code backend family contract changed")
	}
}

func TestClientRegistryReturnsDefensiveCapabilityCopies(t *testing.T) {
	definitions := ClientDefinitions()
	definitions[0].Capabilities.Scopes[0] = ScopeProject
	definitions[0].Capabilities.MCPTransports["stdio"] = SupportUnsupported
	definition, ok := ClientDefinitionFor(ClientCodex)
	if !ok || definition.Capabilities.Scopes[0] != ScopeUser || definition.Capabilities.MCPTransports["stdio"] != SupportProjected {
		t.Fatalf("registry was mutated through returned copy: %+v", definition)
	}
}

func TestNewClientsUsePreparedReadOnlyFoundation(t *testing.T) {
	for _, id := range []ClientID{ClientGemini, ClientOpenCode, ClientCline, ClientWindsurf} {
		definition, ok := ClientDefinitionFor(id)
		if !ok || definition.Capabilities.PackageMode != PackagePrepared || definition.DirectoryDelivery != "prepared" || definition.LegacyCatalogRequired {
			t.Fatalf("new client definition %s = %+v", id, definition)
		}
	}
	claude, ok := ClientDefinitionFor(ClientClaude)
	if !ok || claude.Capabilities.PackageMode != PackageProjection || claude.DirectoryDelivery != "managed" || claude.LegacyCatalogRequired {
		t.Fatalf("Claude Code definition = %+v", claude)
	}
}

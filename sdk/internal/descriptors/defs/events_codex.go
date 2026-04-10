package defs

import "github.com/777genius/plugin-kit-ai/sdk/internal/runtime"

func codexEvents() []EventDescriptor {
	return []EventDescriptor{
		{
			Platform: "codex",
			Event:    "Notify",
			Invocation: InvocationBinding{
				Kind: runtime.InvocationArgvCommandCaseFold,
				Name: "notify",
			},
			Carrier: runtime.CarrierArgvJSON,
			Contract: ContractMeta{
				Maturity: runtime.MaturityStable,
				V1Target: true,
			},
			DecodeFunc: "DecodeNotify",
			EncodeFunc: "EncodeNotify",
			Registrar: RegistrarMeta{
				MethodName:   "OnNotify",
				EventType:    "*NotifyEvent",
				ResponseType: "*Response",
				WrapFunc:     "wrapNotify",
			},
			Docs: DocsMeta{
				SnippetKey: "codex-notify",
				TableGroup: "codex",
				Summary:    "Codex notify hook",
			},
			Capabilities: []runtime.CapabilityID{"notify"},
			CapabilityMappings: []CapabilityMapping{
				{Unified: "notify", Platform: "notify"},
			},
		},
	}
}

package gen

import "github.com/777genius/plugin-kit-ai/sdk/internal/runtime"

func codexSupportEntries() []runtime.SupportEntry {
	return []runtime.SupportEntry{
		{
			Platform:       "codex",
			Event:          "Notify",
			Status:         "runtime_supported",
			Maturity:       "stable",
			V1Target:       true,
			InvocationKind: "argv_command_casefold",
			Carrier:        runtime.CarrierArgvJSON,
			TransportModes: []runtime.TransportMode{
				"process",
			},
			ScaffoldSupport: true,
			ValidateSupport: true,
			Capabilities: []runtime.CapabilityID{
				"notify",
			},
			Summary:         "Codex notify hook",
			LiveTestProfile: "codex_notify",
		},
	}
}

module github.com/plugin-kit-ai/plugin-kit-ai/cli

go 1.23

replace github.com/777genius/plugin-kit-ai/sdk => ../../sdk/plugin-kit-ai

replace github.com/plugin-kit-ai/plugin-kit-ai/plugininstall => ../../install/plugininstall

require (
	github.com/pelletier/go-toml/v2 v2.2.4
	github.com/plugin-kit-ai/plugin-kit-ai/plugininstall v0.0.0
	github.com/777genius/plugin-kit-ai/sdk v0.0.0
	github.com/spf13/cobra v1.10.2
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/tailscale/hujson v0.0.0-20260302212456-ecc657c15afd // indirect
)

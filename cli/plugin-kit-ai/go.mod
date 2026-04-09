module github.com/777genius/plugin-kit-ai/cli

go 1.23

toolchain go1.25.9

replace github.com/777genius/plugin-kit-ai/sdk => ../../sdk

replace github.com/777genius/plugin-kit-ai/install/integrationctl => ../../install/integrationctl

replace github.com/777genius/plugin-kit-ai/plugininstall => ../../install/plugininstall

require (
	github.com/777genius/plugin-kit-ai/install/integrationctl v0.0.0
	github.com/777genius/plugin-kit-ai/plugininstall v0.0.0
	github.com/777genius/plugin-kit-ai/sdk v0.0.0
	github.com/pelletier/go-toml/v2 v2.3.0
	github.com/spf13/cobra v1.10.2
	github.com/tailscale/hujson v0.0.0-20260302212456-ecc657c15afd
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/cpuguy83/go-md2man/v2 v2.0.6 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
)

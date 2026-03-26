module github.com/hookplex/hookplex/cli

go 1.22

replace github.com/hookplex/hookplex/sdk => ../../sdk/hookplex

replace github.com/hookplex/hookplex/plugininstall => ../../install/plugininstall

require (
	github.com/hookplex/hookplex/plugininstall v0.0.0
	github.com/hookplex/hookplex/sdk v0.0.0
	github.com/spf13/cobra v1.10.2
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
)

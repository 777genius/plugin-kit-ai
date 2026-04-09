---
title: "plugin-kit-ai completion zsh"
description: "Generate the autocompletion script for zsh"
canonicalId: "command:plugin-kit-ai:completion:zsh"
surface: "cli"
section: "api"
locale: "zh"
generated: true
editLink: false
stability: "public-stable"
maturity: "stable"
sourceRef: "cli:plugin-kit-ai completion zsh"
translationRequired: false
---
<DocMetaCard surface="cli" stability="public-stable" maturity="stable" source-ref="cli:plugin-kit-ai completion zsh" source-href="https://github.com/777genius/plugin-kit-ai/tree/main/cli/plugin-kit-ai" />

# plugin-kit-ai completion zsh

由实际的 Cobra 命令树生成。

Generate the autocompletion script for zsh

## plugin-kit-ai completion zsh

Generate the autocompletion script for zsh

### Synopsis

Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment you will need
to enable it.  You can execute the following once:

	echo "autoload -U compinit; compinit" &gt;&gt; ~/.zshrc

To load completions in your current shell session:

	source &lt;(plugin-kit-ai completion zsh)

To load completions for every new session, execute once:

#### Linux:

	plugin-kit-ai completion zsh &gt; "${fpath[1]}/_plugin-kit-ai"

#### macOS:

	plugin-kit-ai completion zsh &gt; $(brew --prefix)/share/zsh/site-functions/_plugin-kit-ai

You will need to start a new shell for this setup to take effect.


```
plugin-kit-ai completion zsh [flags]
```

### Options

```
  -h, --help              help for zsh
      --no-descriptions   disable completion descriptions
```

### SEE ALSO

* plugin-kit-ai completion	 - Generate the autocompletion script for the specified shell

---
title: "plugin-kit-ai completion bash"
description: "Generate the autocompletion script for bash"
canonicalId: "command:plugin-kit-ai:completion:bash"
surface: "cli"
section: "api"
locale: "fr"
generated: true
editLink: false
stability: "public-stable"
maturity: "stable"
sourceRef: "cli:plugin-kit-ai completion bash"
translationRequired: false
---
<DocMetaCard surface="cli" stability="public-stable" maturity="stable" source-ref="cli:plugin-kit-ai completion bash" source-href="https://github.com/777genius/plugin-kit-ai/tree/main/cli/plugin-kit-ai" />

# plugin-kit-ai completion bash

Généré à partir de l'arbre réel de commandes Cobra.

Generate the autocompletion script for bash

## plugin-kit-ai completion bash

Generate the autocompletion script for bash

### Synopsis

Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

	source &lt;(plugin-kit-ai completion bash)

To load completions for every new session, execute once:

#### Linux:

	plugin-kit-ai completion bash &gt; /etc/bash_completion.d/plugin-kit-ai

#### macOS:

	plugin-kit-ai completion bash &gt; $(brew --prefix)/etc/bash_completion.d/plugin-kit-ai

You will need to start a new shell for this setup to take effect.


```
plugin-kit-ai completion bash
```

### Options

```
  -h, --help              help for bash
      --no-descriptions   disable completion descriptions
```

### SEE ALSO

* plugin-kit-ai completion	 - Generate the autocompletion script for the specified shell

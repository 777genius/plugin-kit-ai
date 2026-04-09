---
title: "plugin-kit-ai bundle install"
description: "Install a local exported Python/Node bundle into a destination directory"
canonicalId: "command:plugin-kit-ai:bundle:install"
surface: "cli"
section: "api"
locale: "fr"
generated: true
editLink: false
stability: "public-stable"
maturity: "stable"
sourceRef: "cli:plugin-kit-ai bundle install"
translationRequired: false
---
<DocMetaCard surface="cli" stability="public-stable" maturity="stable" source-ref="cli:plugin-kit-ai bundle install" source-href="https://github.com/777genius/plugin-kit-ai/tree/main/cli/plugin-kit-ai" />

# plugin-kit-ai bundle install

Généré à partir de l'arbre réel de commandes Cobra.

Install a local exported Python/Node bundle into a destination directory

## plugin-kit-ai bundle install

Install a local exported Python/Node bundle into a destination directory

### Synopsis

Install a local .tar.gz bundle created by plugin-kit-ai export into a destination directory.

This stable local handoff surface only supports local exported Python/Node bundles for codex-runtime or claude.
It unpacks bundle contents safely, prints next steps, and does not extend the binary-only plugin-kit-ai install flow.

```
plugin-kit-ai bundle install &lt;bundle.tar.gz&gt; [flags]
```

### Options

```
      --dest string   destination directory for unpacked bundle contents
  -f, --force         overwrite an existing destination directory
  -h, --help          help for install
```

### SEE ALSO

* plugin-kit-ai bundle	 - Bundle tooling for exported interpreted-runtime handoff archives

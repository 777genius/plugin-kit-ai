---
title: "plugin-kit-ai skills install"
description: "Install external Agent Skills through the npm skills CLI"
canonicalId: "command:plugin-kit-ai:skills:install"
surface: "cli"
section: "api"
locale: "en"
generated: true
editLink: false
stability: "public-stable"
maturity: "stable"
sourceRef: "cli:plugin-kit-ai skills install"
translationRequired: false
---
<DocMetaCard surface="cli" stability="public-stable" maturity="stable" source-ref="cli:plugin-kit-ai skills install" source-href="https://github.com/777genius/plugin-kit-ai/tree/main/cli/plugin-kit-ai" />

# plugin-kit-ai skills install

Generated from the live Cobra command tree.

Install external Agent Skills through the npm skills CLI

## plugin-kit-ai skills install

Install external Agent Skills through the npm skills CLI

### Synopsis

Install external Agent Skills by forwarding to `npx -y skills@&lt;version&gt; add`.

```
plugin-kit-ai skills install &lt;source&gt; [flags]
```

### Examples

```
  plugin-kit-ai skills install flutter/skills --global --all
  plugin-kit-ai skills install dart-lang/skills --skill '*' --agent codex --agent claude-code --global --yes
  plugin-kit-ai skills add flutter/skills --list
```

### Options

```
  -a, --agent strings               agent(s) to install to, use '*' for all agents
      --all                         shorthand for --skill '*' --agent '*' --yes in the upstream skills CLI
      --copy                        copy files instead of symlinking to agent directories
      --full-depth                  search all subdirectories even when a root SKILL.md exists
  -g, --global                      install skill globally (user-level) instead of project-level
  -h, --help                        help for install
  -l, --list                        list available skills in the repository without installing
  -s, --skill strings               skill name(s) to install, use '*' for all skills
      --skills-cli-version string   npm skills CLI version to run (default "1.5.5")
  -y, --yes                         skip confirmation prompts
```

### SEE ALSO

* plugin-kit-ai skills	 - Experimental skill authoring tools

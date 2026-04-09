---
title: "Installation"
description: "Install plugin-kit-ai using supported channels."
canonicalId: "page:guide:installation"
section: "guide"
locale: "en"
generated: false
translationRequired: true
---

# Installation

Use `npx` for the fastest first plugin install. Use Homebrew when you want plugin-kit-ai installed for daily work.

## Fastest First Plugin Install

```bash
npx plugin-kit-ai@latest add notion --target claude
npx plugin-kit-ai@latest add notion
```

- The first command is the safe single-target path.
- The second installs every supported output for that plugin.

## Supported Channels

- Homebrew for the cleanest default CLI path.
- npm when your environment is already centered around npm.
- PyPI / pipx when your environment is already centered around Python.
- Verified install script as the fallback path.

## Recommended Commands

### Homebrew

```bash
brew install 777genius/homebrew-plugin-kit-ai/plugin-kit-ai
plugin-kit-ai version
```

### npm

```bash
npm i -g plugin-kit-ai
plugin-kit-ai version
```

### PyPI / pipx

```bash
pipx install plugin-kit-ai
plugin-kit-ai version
```

### Verified Script

```bash
curl -fsSL https://raw.githubusercontent.com/777genius/plugin-kit-ai/main/scripts/install.sh | sh
plugin-kit-ai version
```

## Which One Should Most People Use?

- Use `npx` when you want the shortest first run and do not want a permanent install yet.
- Use Homebrew if you are on macOS and want the smoothest daily-use path.
- Use npm or pipx only when that already matches your team environment.
- Use the verified script when you need a fallback outside package-manager-first setups.

## After Install

Most people should continue straight to [Quickstart](/en/guide/quickstart), try a real plugin first, then create the first repo on the job-first path that matches the work.

If you chose `pipx` because your team is Python-first and you already know you want the Python path, continue with [Build A Python Runtime Plugin](/en/guide/python-runtime).

## CI Install Path

For CI, prefer the dedicated setup action instead of teaching every workflow how to download the CLI manually.

## Important Boundary

The npm and PyPI packages are install channels for the CLI. They are not runtime APIs and they are not SDKs.

See [Reference > Install Channels](/en/reference/install-channels) for the contract boundary.

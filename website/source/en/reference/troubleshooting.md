---
title: "Troubleshooting"
description: "The most common failure modes when installing, rendering, validating, or bootstrapping plugin-kit-ai projects."
canonicalId: "page:reference:troubleshooting"
section: "reference"
locale: "en"
generated: false
translationRequired: true
---

# Troubleshooting

## The CLI Installs But Does Not Run

Check that the binary is actually on your shell `PATH`. If you used npm or PyPI to install the CLI, verify that it downloaded the published binary successfully instead of assuming the package itself is the runtime.

## Python Or Node Runtime Projects Fail Early

Check the real runtime first:

- Python runtime projects require Python `3.10+`
- Node runtime projects require Node.js `20+`

Use `plugin-kit-ai doctor <path>` before assuming the project itself is broken.

## `validate --strict` Fails

Treat this as signal, not noise. The point of strict validation is to catch drift or readiness problems before you treat the project as healthy.

Common causes:

- generated artifacts are stale because `render` was skipped
- the selected platform does not match the project source
- the runtime path needs bootstrap or environment fixes

## `render` Output Looks Different Than Expected

That usually means the project source and your mental model have drifted apart. Re-check the package-standard layout instead of hand-editing generated target files to “fix” the output.

## I Am Unsure Which Path I Should Use

Start with the default Go path if you want the strongest contract. Move to Node/TypeScript or Python only when the repo-local runtime tradeoff is real and intentional.

See [Authoring Workflow](/en/reference/authoring-workflow) and [FAQ](/en/reference/faq).

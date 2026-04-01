---
title: "FAQ"
description: "Common questions about choosing paths, wrappers, targets, and authoring flows."
canonicalId: "page:reference:faq"
section: "reference"
locale: "en"
generated: false
translationRequired: true
---

# FAQ

## Should I Start With Go, Python, Or Node?

Start with Go unless you have a real reason not to. Choose Node/TypeScript for the main supported non-Go path. Choose Python when the plugin stays local to the repo and your team is already Python-first.

## Are npm And PyPI `plugin-kit-ai` Packages Runtime APIs?

No. They are ways to install the CLI. They are not public runtime APIs and they are not SDKs.

## When Should I Use Bundle Commands?

Use bundle commands when you need portable Python or Node artifacts that another machine can fetch or install. Do not confuse them with the main CLI install path.

## Can I Keep Native Target Files As My Source Of Truth?

That is not the intended long-term model. The project source of truth should live in the package-standard layout, and target files should be generated outputs.

## Is `render` Optional?

Not if you want the managed project model. `render` is part of the workflow, not an optional extra.

## Is `validate --strict` Optional?

Treat it as the main readiness check, especially for local Python and Node runtime projects.

## Can One Repo Own Multiple Targets?

Yes. That is one of the main ideas in `plugin-kit-ai`.

The practical rule is:

- keep the authored state in one managed repo
- start with the primary target you need today
- add other targets when real product, delivery, or integration requirements appear

See [One Project, Multiple Targets](/en/guide/one-project-multiple-targets) and [Target Model](/en/concepts/target-model).

## Are All Targets Equally Stable?

No. Runtime, packaging, extension, and workspace-configuration targets do not all carry the same support promise.

See [Support Boundary](/en/reference/support-boundary), [Target Support](/en/reference/target-support), and [Authoring Workflow](/en/reference/authoring-workflow).

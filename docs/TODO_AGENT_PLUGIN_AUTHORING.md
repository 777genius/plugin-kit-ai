# TODO: Agent Plugin authoring experience

The preserved `/create-plugin` page still describes the legacy `plugin.yaml` authoring workflow. Do not publish it as current Agent Plugins 1.0 guidance until the product owner approves the authoring contract.

Before changing the public authoring experience, evaluate these options:

1. Make the standard `plugin.json` the only authoring source.
2. Make `plugin.json` primary and allow an optional, non-installed build/publish sidecar.
3. Keep `plugin.yaml` only as an explicit legacy migration input that generates a standard package.

The decision must preserve these invariants:

- installed packages are driven by Agent Plugins 1.0 `plugin.json`;
- build or publishing metadata cannot silently override the standard manifest;
- generated output is deterministic and reviewable;
- existing legacy users receive a documented migration path.

No authoring-format decision or public documentation rewrite should be made without explicit owner approval.

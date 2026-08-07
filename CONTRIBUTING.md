# Contributing

Contributions that improve portability, validation, documentation, or an
existing package are welcome.

## Add or update a plugin

1. Use a folder under `plugins/<plugin-name>/`.
2. Add a root `plugin.json` targeting the Agent Plugins 1.0.0 schema.
3. Add `mcp.json` only for a vendor-documented MCP endpoint or a bundled stdio
   server with a reproducible dependency pin.
4. Put Agent Skills only in immediate child folders under `skills/`.
5. Add a package `README.md` with upstream documentation, authentication, scope,
   and known write capabilities.
6. Update `SOURCES.md` and `docs/COMPATIBILITY.md`.
7. Run the validator and unit tests.

Do not add credentials, token placeholders in remote headers, undocumented OAuth
metadata, shell command strings, floating dependency tags, or endpoints copied
from a client-specific hosted connector.

Package descriptions must say `Community package for` rather than implying that
777genius authored, owns, or is endorsed by the upstream service.

## Pull requests

Keep pull requests focused. Explain the upstream source, what changed, how the
package was validated, and whether authentication or tool permissions changed.
Use conventional commit titles such as `feat: add example plugin` or
`fix: update context7 server pin`.

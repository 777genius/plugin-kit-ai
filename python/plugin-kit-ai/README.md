# plugin-kit-ai (PyPI wrapper)

Thin Python launcher for the `plugin-kit-ai` CLI.

It downloads the matching published GitHub Releases binary, verifies
`checksums.txt`, caches the binary locally, and executes it.

Primary user path:

```bash
pipx install plugin-kit-ai
plugin-kit-ai version
```

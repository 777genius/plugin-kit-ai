## External registry submission

Exact install source: `owner/repo@FULL_40_CHARACTER_SHA//path/to/plugin`

<!-- Explain the package purpose, authentication, permissions, and known write/destructive capabilities. -->

## Checklist

- [ ] The descriptor contains only source metadata and pins a full commit SHA.
- [ ] The package name, author, source, and license come from the pinned package.
- [ ] I ran `python3 scripts/build_registry.py --check` after generation.
- [ ] I tested the exact immutable install source shown above.
- [ ] I understand schema-only validation is not verification or endorsement.
- [ ] I understand that updates require another pull request pinned to a new commit.

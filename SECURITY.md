# Security

For vulnerability reports, use GitHub Security Advisories if they are enabled
for this repository, or contact the maintainers privately. Do not open a
public issue for undisclosed security problems.

When reviewing plugin behavior, treat hook stdin, path-like fields, tool
arguments, prompts, and any generated native config as untrusted input until it
has been validated for the target runtime.

Security-sensitive release checks before publishing:

- verify release assets are built from the intended candidate SHA
- keep `checksums.txt` attached to the root GitHub release
- verify release attestations for published artifacts with `gh attestation verify`
- verify Homebrew, npm, and PyPI channels consume the same release assets
- keep `dependency-review`, `govulncheck`, and `CodeQL` green on the candidate branch
- record any waived live failures in the release notes with scope and reason

# Security policy

## Report a vulnerability

Do not open a public issue for leaked credentials, command execution, path
escape, malicious skill instructions, or a package that connects to an
unexpected endpoint. Contact the repository owner privately through the
security-reporting channel configured on the GitHub repository.

Never include live tokens, cookies, personal data, or production credentials in
a report. Use redacted logs and a minimal reproduction.

## Scope

This repository packages configuration and skills. It does not operate the
linked vendor MCP servers and cannot fix upstream service vulnerabilities.
Reports about an upstream server should also follow that vendor's security
policy.

Agent Plugins 1.0 does not define a permission system, signature verification,
sandboxing, or a portable secret store. Users must review every server's tools
and configure authentication through their client.

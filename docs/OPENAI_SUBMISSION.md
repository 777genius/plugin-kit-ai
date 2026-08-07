# OpenAI Plugins Directory submission

Conformance to Agent Plugins 1.0 and inclusion in OpenAI's public Plugins
Directory are separate milestones.

The generated OpenAI adapters in this repository are suitable for local package
validation. They are not submission-ready public listings.

## Requirements per public plugin

- Apps Management write access in the publishing OpenAI organization.
- Verified individual or business identity matching the public publisher.
- Production listing name, descriptions, category, square logo, website,
  support page, privacy policy, and terms of service.
- For MCP plugins, a production HTTPS server URL controlled or legitimately
  submitted by the publisher.
- Domain verification at the portal-provided
  `/.well-known/openai-apps-challenge` location.
- Accurate tool schemas and annotations for read-only, open-world, and
  destructive behavior.
- Reviewer credentials that work without MFA, email confirmation, SMS, or a
  private network when authentication is required.
- Exactly five positive and three negative reviewer test cases as the safe
  interpretation of current validator guidance.
- Starter prompts, availability regions, release notes, and policy
  attestations.

## Vendor-owned MCP limitation

Most packages here point at MCP servers operated by another vendor. A community
repository can package those documented endpoints for compatible clients, but
it must not claim ownership, replace the vendor's verification token, or submit
an existing integration reference as if it were a new first-party service.

The safe public-directory paths are:

1. the upstream vendor publishes the plugin;
2. the vendor explicitly authorizes a joint submission;
3. a skills-only community plugin is submitted without misrepresenting server
   ownership; or
4. a separately operated MCP service is submitted with its own domain,
   policies, support, and review materials.

Do not create a proxy merely to bypass domain ownership or existing integration
rules.

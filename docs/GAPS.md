# Known gaps

The legacy catalog contains four client-specific hosted connectors that are not
portable Agent Plugins MCP configurations:

| Legacy package | Why it was not copied | Recommended path |
| --- | --- | --- |
| `gmail` | Points to an Anthropic-hosted `claude.com` MCP endpoint | Install the Gmail plugin or connector provided by the target client |
| `google-calendar` | Points to an Anthropic-hosted `claude.com` MCP endpoint | Install the Google Calendar plugin or connector provided by the target client |
| `microsoft365` | Combines Outlook, Teams, SharePoint, and OneDrive through an Anthropic-hosted endpoint | Use the target client's separate Outlook, Teams, and SharePoint plugins |
| `slack` | Contains client-specific OAuth metadata that Agent Plugins 1.0 cannot represent portably | Install the target client's Slack plugin or use a vendor-documented portable endpoint when available |

Agent Plugins 1.0 intentionally has no portable OAuth or secret-reference
fields. Copying those old endpoints or OAuth client IDs would produce packages
that look universal but are not.

Google Drive was documented as a gap in the legacy catalog and therefore is not
part of this migration.

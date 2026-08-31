import type { ClientTarget, RegistryPlugin } from '~/types/registry'
import { githubSourceUrl, mirroredIconPath } from '~/utils/registry'

export const clients: ClientTarget[] = [
  { id: 'codex', name: 'Codex', icon: 'openai.svg', note: 'Skills and supported MCP transports' },
  { id: 'chatgpt', name: 'ChatGPT', icon: 'openai.svg', note: 'Registered remote MCP app paths' },
  { id: 'cursor', name: 'Cursor', icon: 'cursor.svg', note: 'Native Agent Plugin package' },
  { id: 'copilot', name: 'GitHub Copilot CLI', icon: 'github-copilot.svg', note: 'Managed native plugin' },
  { id: 'vscode', name: 'VS Code', icon: 'vscode.svg', note: 'Copilot plugin integration' },
  { id: 'kiro', name: 'Kiro', icon: 'kiro.svg', note: 'Native folder package' },
  { id: 'claude', name: 'Claude Code', icon: 'terminal.svg', note: 'Managed MCP configuration' },
  { id: 'gemini', name: 'Gemini CLI', icon: 'terminal.svg', note: 'Managed MCP configuration' },
  { id: 'opencode', name: 'OpenCode', icon: 'terminal.svg', note: 'Managed MCP configuration' },
  { id: 'cline', name: 'Cline', icon: 'terminal.svg', note: 'Managed MCP configuration' },
  { id: 'windsurf', name: 'Windsurf', icon: 'terminal.svg', note: 'Prepared package; manual activation required' },
]

export function useSite() {
  const config = useRuntimeConfig()
  const baseURL = String(config.public.baseURL)
  const repositoryUrl = String(config.public.repositoryUrl)
  const asset = (path: string) => `${baseURL}${path.replace(/^\//, '')}`

  const pluginIcon = (plugin: RegistryPlugin) => {
    // External author-controlled images are never loaded. A future sanitized
    // mirror can opt entries into locally served assets explicitly.
    const path = mirroredIconPath(plugin)
    return asset(path ?? 'logo.svg')
  }

  const sourceUrl = (plugin: RegistryPlugin) => {
    return githubSourceUrl(plugin)
  }

  return { asset, baseURL, pluginIcon, repositoryUrl, sourceUrl }
}

import type { ClientTarget, RegistryPlugin } from '~/types/registry'
import { githubSourceUrl, mirroredIconPath } from '~/utils/registry'

export const clients: ClientTarget[] = [
  { id: 'codex', name: 'Codex', icon: 'openai.svg', note: 'Skills and supported MCP transports', status: 'CLI supported' },
  { id: 'chatgpt', name: 'ChatGPT', icon: 'openai.svg', note: 'Verified ChatGPT connections', status: 'App setup' },
  { id: 'cursor', name: 'Cursor', icon: 'cursor.svg', note: 'Native Agent Plugin package', status: 'CLI supported' },
  { id: 'copilot', name: 'GitHub Copilot CLI', icon: 'github-copilot.svg', note: 'Managed native plugin', status: 'CLI supported' },
  { id: 'vscode', name: 'VS Code', icon: 'vscode.svg', note: 'Copilot plugin integration', status: 'CLI supported' },
  { id: 'kiro', name: 'Kiro', icon: 'kiro.svg', note: 'Native folder package', status: 'CLI supported' },
  { id: 'claude', name: 'Claude Code', icon: 'claude.svg', note: 'Managed MCP configuration', status: 'CLI supported' },
  { id: 'gemini', name: 'Gemini CLI', icon: 'gemini.svg', note: 'Managed MCP configuration', status: 'CLI supported' },
  { id: 'opencode', name: 'OpenCode', icon: 'opencode.svg', note: 'Managed MCP configuration', status: 'CLI supported' },
  { id: 'cline', name: 'Cline', icon: 'cline.svg', note: 'Managed MCP configuration', status: 'CLI supported' },
  { id: 'windsurf', name: 'Windsurf', icon: 'windsurf.svg', note: 'Prepared package; manual activation required', status: 'CLI prepares' },
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
    return path ? asset(path) : undefined
  }

  const sourceUrl = (plugin: RegistryPlugin) => {
    return githubSourceUrl(plugin)
  }

  return { asset, baseURL, pluginIcon, repositoryUrl, sourceUrl }
}

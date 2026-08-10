import type { ClientTarget, RegistryPlugin } from '~/types/registry'

export const clients: ClientTarget[] = [
  { id: 'codex', name: 'Codex', icon: 'openai.svg', note: 'Skills and supported MCP transports' },
  { id: 'chatgpt', name: 'ChatGPT', icon: 'openai.svg', note: 'Registered remote MCP app paths' },
  { id: 'cursor', name: 'Cursor', icon: 'cursor.svg', note: 'Native Agent Plugin package' },
  { id: 'copilot', name: 'GitHub Copilot CLI', icon: 'github-copilot.svg', note: 'Managed native plugin' },
  { id: 'vscode', name: 'VS Code', icon: 'vscode.svg', note: 'Copilot plugin integration' },
  { id: 'kiro', name: 'Kiro', icon: 'kiro.svg', note: 'Native folder package' },
]

export function useSite() {
  const config = useRuntimeConfig()
  const baseURL = String(config.public.baseURL)
  const repositoryUrl = String(config.public.repositoryUrl)
  const asset = (path: string) => `${baseURL}${path.replace(/^\//, '')}`

  const pluginIcon = (plugin: RegistryPlugin) => {
    if (!plugin.icon) return asset('logo.svg')
    const filename = plugin.icon.split('/').at(-1)
    return filename ? asset(`plugin-icons/${filename}`) : asset('logo.svg')
  }

  const sourceUrl = (plugin: RegistryPlugin) => {
    if (/^https:\/\//.test(plugin.source)) return plugin.source
    if (!plugin.built_in) {
      const match = plugin.install_source.match(/^([^@]+)@([a-f0-9]{40})\/\/(.+)$/)
      if (match) return `https://github.com/${match[1]}/tree/${match[2]}/${match[3]}`
    }
    return `${repositoryUrl}/tree/main/${plugin.source.replace(/^\.\//, '')}`
  }

  return { asset, baseURL, pluginIcon, repositoryUrl, sourceUrl }
}

export interface PluginAuthor {
  name: string
  email?: string
  url?: string
}

export interface PluginSource {
  repository: string
  revision: string
  path: string
  manifest_sha256: string
  tree_sha256: string
  icon_sha256?: string
}

export interface PluginValidation {
  level: 'schema_only' | 'runtime_evidence'
  schema: 'agent-plugins-1.0'
  runtime_evidence: string[]
}

export interface PluginIcon {
  path: string
  sha256: string
}

export type ClientID = 'codex' | 'chatgpt' | 'cursor' | 'copilot' | 'vscode' | 'kiro'

export interface PluginClientSupport {
  resolution: 'catalog' | 'install_time'
  clients: ClientID[]
}

export interface RegistryPlugin {
  name: string
  version: string
  description: string
  author: PluginAuthor
  license: string
  categories: string[]
  keywords: string[]
  source: PluginSource
  install_source: string
  built_in: boolean
  client_support: PluginClientSupport
  validation: PluginValidation
  components: Array<'extensions' | 'mcp' | 'skills'>
  icon?: PluginIcon
}

export interface RegistryIndex {
  schema_version: 1
  plugins: RegistryPlugin[]
}

export interface ClientTarget {
  id: ClientID
  name: string
  icon: string
  note: string
}

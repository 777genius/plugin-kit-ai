export interface PluginValidation {
  label: string
  schemaValidated: boolean
  runtimeTested: boolean
}

export interface RegistryPlugin {
  name: string
  version: string
  description: string
  author: string
  license: string
  categories: string[]
  keywords: string[]
  source: string
  install_source: string
  built_in: boolean
  validation: PluginValidation
  components: string[]
  icon?: string
}

export interface RegistryIndex {
  schema_version: 1
  plugins: RegistryPlugin[]
}

export interface ClientTarget {
  id: 'codex' | 'chatgpt' | 'cursor' | 'copilot' | 'vscode' | 'kiro'
  name: string
  icon: string
  note: string
}

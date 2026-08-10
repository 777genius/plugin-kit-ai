import type { ClientID, PluginAuthor, PluginClientSupport, PluginIcon, PluginSource, PluginValidation, RegistryIndex, RegistryPlugin } from '../types/registry'

const REPOSITORY = /^[a-z0-9](?:[a-z0-9-]{0,37}[a-z0-9])?\/[a-z0-9](?:[a-z0-9._-]{0,98}[a-z0-9])?$/
const REVISION = /^[a-f0-9]{40}$/
const DIGEST = /^sha256:[a-f0-9]{64}$/
const PLUGIN_NAME = /^(?!.*(?:--|\.\.))[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$/
const COMPONENTS = new Set(['extensions', 'mcp', 'skills'])
const CLIENTS = new Set<ClientID>(['codex', 'chatgpt', 'cursor', 'copilot', 'vscode', 'kiro'])

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function requiredString(item: Record<string, unknown>, field: string, context: string): string {
  const value = item[field]
  if (typeof value !== 'string' || value.length === 0) {
    throw new Error(`${context}: ${field} must be a non-empty string`)
  }
  return value
}

function stringArray(value: unknown, field: string, context: string): string[] {
  if (!Array.isArray(value) || value.some(entry => typeof entry !== 'string' || entry.length === 0)) {
    throw new Error(`${context}: ${field} must be an array of non-empty strings`)
  }
  const values = value as string[]
  if (new Set(values).size !== values.length) throw new Error(`${context}: ${field} must be unique`)
  return values
}

function digest(item: Record<string, unknown>, field: string, context: string): string {
  const value = requiredString(item, field, context)
  if (!DIGEST.test(value)) throw new Error(`${context}: ${field} must be a sha256 digest`)
  return value
}

function author(value: unknown, context: string): PluginAuthor {
  if (!record(value)) throw new Error(`${context}: author must be an object`)
  const result: PluginAuthor = { name: requiredString(value, 'name', `${context} author`) }
  for (const field of ['email', 'url'] as const) {
    if (value[field] !== undefined) result[field] = requiredString(value, field, `${context} author`)
  }
  return result
}

function source(value: unknown, context: string): PluginSource {
  if (!record(value)) throw new Error(`${context}: source must be an object`)
  const repository = requiredString(value, 'repository', `${context} source`)
  const revision = requiredString(value, 'revision', `${context} source`)
  if (!REPOSITORY.test(repository)) throw new Error(`${context}: source repository is invalid`)
  if (!REVISION.test(revision)) throw new Error(`${context}: source revision must be a full commit SHA`)
  const result: PluginSource = {
    repository,
    revision,
    path: requiredString(value, 'path', `${context} source`),
    manifest_sha256: digest(value, 'manifest_sha256', `${context} source`),
    tree_sha256: digest(value, 'tree_sha256', `${context} source`),
  }
  if (value.icon_sha256 !== undefined) result.icon_sha256 = digest(value, 'icon_sha256', `${context} source`)
  return result
}

function validation(value: unknown, context: string): PluginValidation {
  if (!record(value)) throw new Error(`${context}: validation must be an object`)
  const level = value.level
  if (level !== 'schema_only' && level !== 'runtime_evidence') throw new Error(`${context}: validation level is invalid`)
  if (value.schema !== 'agent-plugins-1.0') throw new Error(`${context}: validation schema is invalid`)
  const runtimeEvidence = stringArray(value.runtime_evidence, 'runtime_evidence', `${context} validation`)
  if ((level === 'schema_only') !== (runtimeEvidence.length === 0)) {
    throw new Error(`${context}: validation level does not match runtime evidence`)
  }
  return { level, schema: 'agent-plugins-1.0', runtime_evidence: runtimeEvidence }
}

function icon(value: unknown, context: string): PluginIcon {
  if (!record(value)) throw new Error(`${context}: icon must be an object`)
  return { path: requiredString(value, 'path', `${context} icon`), sha256: digest(value, 'sha256', `${context} icon`) }
}

function clientSupport(value: unknown, builtIn: boolean, context: string): PluginClientSupport {
  if (!record(value)) throw new Error(`${context}: client_support must be an object`)
  const resolution = value.resolution
  if (resolution !== 'catalog' && resolution !== 'install_time') throw new Error(`${context}: client support resolution is invalid`)
  if ((builtIn && resolution !== 'catalog') || (!builtIn && resolution !== 'install_time')) {
    throw new Error(`${context}: client support resolution does not match source type`)
  }
  const clientIDs = stringArray(value.clients, 'clients', `${context} client support`)
  if (!clientIDs.length || clientIDs.some(client => !CLIENTS.has(client as ClientID))) throw new Error(`${context}: client support contains an invalid client`)
  return { resolution, clients: clientIDs as ClientID[] }
}

export function parseRegistryIndex(input: unknown): RegistryIndex {
  if (!record(input) || input.schema_version !== 1 || !Array.isArray(input.plugins)) {
    throw new Error('registry index must have schema_version 1 and a plugins array')
  }

  const names = new Set<string>()
  const plugins = input.plugins.map((raw, index): RegistryPlugin => {
    const context = `registry plugin ${index}`
    if (!record(raw)) throw new Error(`${context}: item must be an object`)
    const name = requiredString(raw, 'name', context)
    if (!PLUGIN_NAME.test(name)) throw new Error(`${context}: invalid name ${name}`)
    const foldedName = name.toLocaleLowerCase('en-US')
    if (names.has(foldedName)) throw new Error(`${context}: duplicate name ${name}`)
    names.add(foldedName)
    if (typeof raw.built_in !== 'boolean') throw new Error(`${context}: built_in must be a boolean`)
    const parsedSource = source(raw.source, context)
    const installSource = requiredString(raw, 'install_source', context)
    const expectedSource = `${parsedSource.repository}@${parsedSource.revision}//${parsedSource.path}`
    if (raw.built_in ? installSource !== name : installSource !== expectedSource) {
      throw new Error(`${context}: ${raw.built_in ? 'built-in install_source must equal its name' : 'external install_source must use source repository@40-char-sha//path'}`)
    }
    const parsedComponents = stringArray(raw.components, 'components', context)
    if (parsedComponents.some(component => !COMPONENTS.has(component))) throw new Error(`${context}: unsupported component`)

    return {
      name,
      version: requiredString(raw, 'version', context),
      description: requiredString(raw, 'description', context),
      author: author(raw.author, context),
      license: requiredString(raw, 'license', context),
      categories: stringArray(raw.categories, 'categories', context),
      keywords: stringArray(raw.keywords, 'keywords', context),
      source: parsedSource,
      install_source: installSource,
      built_in: raw.built_in,
      client_support: clientSupport(raw.client_support, raw.built_in, context),
      validation: validation(raw.validation, context),
      components: parsedComponents as RegistryPlugin['components'],
      ...(raw.icon === undefined ? {} : { icon: icon(raw.icon, context) }),
    }
  })

  return { schema_version: 1, plugins }
}

export function isPinnedExternalSource(value: string): boolean {
  const match = /^([^@]+)@([a-f0-9]{40})\/\/(.+)$/.exec(value)
  return Boolean(match && REPOSITORY.test(match[1]!) && match[3]!.length > 0)
}

export function validationLabel(value: PluginValidation): string {
  return value.level === 'runtime_evidence' ? 'Runtime evidence' : 'Schema validated'
}

export function githubSourceUrl(plugin: RegistryPlugin): string {
  const path = plugin.source.path.split('/').map(encodeURIComponent).join('/')
  return `https://github.com/${plugin.source.repository}/tree/${plugin.source.revision}/${path}`
}

export function mirroredIconPath(plugin: RegistryPlugin): string | undefined {
  if (!plugin.built_in || !plugin.icon || !plugin.icon.path.startsWith('assets/plugin-icons/')) return undefined
  const filename = plugin.icon.path.split('/').at(-1)
  return filename ? `plugin-icons/${filename}` : undefined
}

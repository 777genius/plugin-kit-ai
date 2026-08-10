import { readFileSync } from 'node:fs'
import type { PluginValidation, RegistryIndex, RegistryPlugin } from '../types/registry'

const EXTERNAL_SOURCE = /^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+@[a-f0-9]{40}\/\/[A-Za-z0-9._/-]+$/
const PLUGIN_NAME = /^[a-z0-9]+(?:-[a-z0-9]+)*$/

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function requiredString(item: Record<string, unknown>, field: string, index: number): string {
  const value = item[field]
  if (typeof value !== 'string' || value.trim() === '') {
    throw new Error(`registry plugin ${index}: ${field} must be a non-empty string`)
  }
  return value.trim()
}

function optionalStrings(value: unknown, field: string, index: number): string[] {
  if (value === undefined) return []
  if (!Array.isArray(value) || value.some(entry => typeof entry !== 'string' || entry.trim() === '')) {
    throw new Error(`registry plugin ${index}: ${field} must be an array of non-empty strings`)
  }
  return [...new Set(value.map(entry => (entry as string).trim()))]
}

function components(value: unknown, index: number): string[] {
  if (Array.isArray(value)) return optionalStrings(value, 'components', index)
  if (record(value)) {
    return Object.entries(value)
      .filter(([, enabled]) => enabled === true || (Array.isArray(enabled) && enabled.length > 0))
      .map(([name]) => name)
  }
  throw new Error(`registry plugin ${index}: components must be an array or component map`)
}

function validation(value: unknown, index: number): PluginValidation {
  if (typeof value === 'string' && value.trim()) {
    const normalized = value.toLowerCase().replaceAll('-', '_').replaceAll(' ', '_')
    return {
      label: normalized.includes('schema') ? 'Schema validated' : value.trim(),
      schemaValidated: normalized.includes('schema') || normalized === 'valid',
      runtimeTested: normalized.includes('runtime_tested'),
    }
  }
  if (value === true) {
    return { label: 'Schema validated', schemaValidated: true, runtimeTested: false }
  }
  if (record(value)) {
    const status = typeof value.status === 'string' ? value.status.toLowerCase() : ''
    const schemaValidated = value.schema_validated === true
      || value.schemaValidated === true
      || status.includes('schema')
      || status === 'valid'
    const runtimeTested = value.runtime_tested === true || value.runtimeTested === true
    return {
      label: schemaValidated ? 'Schema validated' : 'Validation reported',
      schemaValidated,
      runtimeTested,
    }
  }
  throw new Error(`registry plugin ${index}: validation must describe validation status`)
}

export function parseRegistryIndex(input: unknown): RegistryIndex {
  if (!record(input) || input.schema_version !== 1 || !Array.isArray(input.plugins)) {
    throw new Error('registry index must have schema_version 1 and a plugins array')
  }

  const names = new Set<string>()
  const plugins = input.plugins.map((raw, index): RegistryPlugin => {
    if (!record(raw)) throw new Error(`registry plugin ${index}: item must be an object`)
    const name = requiredString(raw, 'name', index)
    if (!PLUGIN_NAME.test(name)) throw new Error(`registry plugin ${index}: invalid name ${name}`)
    if (names.has(name)) throw new Error(`registry plugin ${index}: duplicate name ${name}`)
    names.add(name)
    if (typeof raw.built_in !== 'boolean') {
      throw new Error(`registry plugin ${index}: built_in must be a boolean`)
    }
    const installSource = requiredString(raw, 'install_source', index)
    if (!raw.built_in && !EXTERNAL_SOURCE.test(installSource)) {
      throw new Error(`registry plugin ${index}: external install_source must use owner/repo@40-char-sha//path`)
    }
    const icon = raw.icon === undefined ? undefined : requiredString(raw, 'icon', index)

    return {
      name,
      version: requiredString(raw, 'version', index),
      description: requiredString(raw, 'description', index),
      author: requiredString(raw, 'author', index),
      license: typeof raw.license === 'string' ? raw.license.trim() : '',
      categories: optionalStrings(raw.categories, 'categories', index),
      keywords: optionalStrings(raw.keywords, 'keywords', index),
      source: requiredString(raw, 'source', index),
      install_source: installSource,
      built_in: raw.built_in,
      validation: validation(raw.validation, index),
      components: components(raw.components, index),
      ...(icon ? { icon } : {}),
    }
  })

  return { schema_version: 1, plugins }
}

export function loadRegistryIndex(path: string): RegistryIndex {
  let source: string
  try {
    source = readFileSync(path, 'utf8')
  } catch (error) {
    throw new Error(`Unable to read registry index at ${path}: ${String(error)}`)
  }
  try {
    return parseRegistryIndex(JSON.parse(source) as unknown)
  } catch (error) {
    throw new Error(`Invalid registry index at ${path}: ${String(error)}`)
  }
}

export function isPinnedExternalSource(value: string): boolean {
  return EXTERNAL_SOURCE.test(value)
}

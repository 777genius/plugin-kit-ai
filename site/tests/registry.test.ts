import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { isPinnedExternalSource, parseRegistryIndex } from '../utils/registry'

const fixture = JSON.parse(readFileSync(fileURLToPath(new URL('./fixtures/registry.valid.json', import.meta.url)), 'utf8')) as unknown

describe('registry parsing', () => {
  it('normalizes valid built-in and external entries', () => {
    const registry = parseRegistryIndex(fixture)
    expect(registry.schema_version).toBe(1)
    expect(registry.plugins).toHaveLength(2)
    expect(registry.plugins[0]?.validation).toEqual({
      label: 'Schema validated',
      schemaValidated: true,
      runtimeTested: true,
    })
    expect(registry.plugins[1]?.components).toEqual(['skills'])
  })

  it('fails on invalid essential fields', () => {
    expect(() => parseRegistryIndex({ schema_version: 1, plugins: [{ name: 'missing-fields' }] }))
      .toThrow('version must be a non-empty string')
    expect(() => parseRegistryIndex({ schema_version: 2, plugins: [] }))
      .toThrow('schema_version 1')
  })

  it('rejects duplicate names', () => {
    const registry = parseRegistryIndex(fixture)
    expect(() => parseRegistryIndex({ schema_version: 1, plugins: [registry.plugins[0], registry.plugins[0]] }))
      .toThrow('duplicate name')
  })
})

describe('external pinned-source behavior', () => {
  const valid = 'owner/repo@0123456789abcdef0123456789abcdef01234567//plugins/example'

  it('recognizes only full 40-character GitHub pins', () => {
    expect(isPinnedExternalSource(valid)).toBe(true)
    expect(isPinnedExternalSource('owner/repo@main//plugins/example')).toBe(false)
    expect(isPinnedExternalSource('example-external')).toBe(false)
  })

  it('fails closed instead of allowing external short-name resolution', () => {
    const registry = parseRegistryIndex(fixture)
    const external = registry.plugins[1]!
    expect(() => parseRegistryIndex({
      schema_version: 1,
      plugins: [{ ...external, install_source: external.name }],
    })).toThrow('external install_source must use owner/repo@40-char-sha//path')
  })
})

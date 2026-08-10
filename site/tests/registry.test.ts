import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { describe, it } from 'node:test'
import { fileURLToPath } from 'node:url'
import { githubSourceUrl, isPinnedExternalSource, mirroredIconPath, parseRegistryIndex, validationLabel } from '../utils/registry.ts'

const fixture = JSON.parse(readFileSync(fileURLToPath(new URL('./fixtures/registry.valid.json', import.meta.url)), 'utf8')) as unknown

describe('registry parsing', () => {
  it('normalizes valid built-in and external entries', () => {
    const registry = parseRegistryIndex(fixture)
    assert.equal(registry.schema_version, 1)
    assert.equal(registry.plugins.length, 2)
    assert.equal(registry.plugins[0]?.author.name, 'Community package for Upstash')
    assert.equal(registry.plugins[0]?.source.path, 'plugins/context7')
    assert.deepEqual(registry.plugins[0]?.client_support.clients, ['codex', 'cursor', 'copilot', 'vscode', 'kiro'])
    assert.equal(registry.plugins[1]?.client_support.resolution, 'install_time')
    assert.deepEqual(registry.plugins[0]?.validation.runtime_evidence, ['codex', 'cursor'])
    assert.equal(validationLabel(registry.plugins[0]!.validation), 'Runtime evidence')
    assert.deepEqual(registry.plugins[1]?.components, ['skills'])
  })

  it('fails on invalid essential fields', () => {
    assert.throws(() => parseRegistryIndex({ schema_version: 1, plugins: [{ name: 'missing-fields' }] }), /built_in must be a boolean/)
    assert.throws(() => parseRegistryIndex({ schema_version: 2, plugins: [] }), /schema_version 1/)
  })

  it('rejects invalid or source-mismatched client support', () => {
    const registry = parseRegistryIndex(fixture)
    const builtIn = registry.plugins[0]!
    assert.throws(() => parseRegistryIndex({
      schema_version: 1,
      plugins: [{ ...builtIn, client_support: { resolution: 'install_time', clients: ['cursor'] } }],
    }), /does not match source type/)
    assert.throws(() => parseRegistryIndex({
      schema_version: 1,
      plugins: [{ ...builtIn, client_support: { resolution: 'catalog', clients: ['unknown'] } }],
    }), /invalid client/)
  })

  it('rejects duplicate names', () => {
    const registry = parseRegistryIndex(fixture)
    assert.throws(() => parseRegistryIndex({ schema_version: 1, plugins: [registry.plugins[0], registry.plugins[0]] }), /duplicate name/)
  })

  it('parses the authoritative generated 26-entry production index', () => {
    const real = JSON.parse(readFileSync(fileURLToPath(new URL('../../registry/index.json', import.meta.url)), 'utf8')) as unknown
    const registry = parseRegistryIndex(real)
    assert.equal(registry.plugins.length, 26)
    assert.equal(registry.plugins.filter(plugin => plugin.built_in).length, 26)
    for (const plugin of registry.plugins.filter(plugin => plugin.icon)) {
      const filename = plugin.icon!.path.split('/').at(-1)!
      const body = readFileSync(fileURLToPath(new URL(`../public/plugin-icons/${filename}`, import.meta.url)))
      assert.equal(`sha256:${createHash('sha256').update(body).digest('hex')}`, plugin.icon!.sha256)
    }
  })

  it('builds immutable GitHub source links and never mirrors external icons', () => {
    const registry = parseRegistryIndex(fixture)
    assert.equal(githubSourceUrl(registry.plugins[1]!), 'https://github.com/example/plugins/tree/0123456789abcdef0123456789abcdef01234567/plugins/example')
    assert.equal(mirroredIconPath(registry.plugins[0]!), 'plugin-icons/context7.png')
    assert.equal(mirroredIconPath({ ...registry.plugins[1]!, icon: registry.plugins[0]!.icon }), undefined)
  })
})

describe('external pinned-source behavior', () => {
  const valid = 'owner/repo@0123456789abcdef0123456789abcdef01234567//plugins/example'

  it('recognizes only full 40-character GitHub pins', () => {
    assert.equal(isPinnedExternalSource(valid), true)
    assert.equal(isPinnedExternalSource('owner/repo@main//plugins/example'), false)
    assert.equal(isPinnedExternalSource('example-external'), false)
  })

  it('fails closed instead of allowing external short-name resolution', () => {
    const registry = parseRegistryIndex(fixture)
    const external = registry.plugins[1]!
    assert.throws(() => parseRegistryIndex({
      schema_version: 1,
      plugins: [{ ...external, install_source: external.name }],
    }), /external install_source must use source repository@40-char-sha\/\/path/)
  })
})

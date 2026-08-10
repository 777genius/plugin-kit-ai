import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { availableFilters, filterPlugins } from '../utils/filter'
import { parseRegistryIndex } from '../utils/registry'

const fixture = JSON.parse(readFileSync(fileURLToPath(new URL('./fixtures/registry.valid.json', import.meta.url)), 'utf8')) as unknown
const plugins = parseRegistryIndex(fixture).plugins

describe('catalog filtering', () => {
  it('searches names, descriptions, authors, keywords, and components case-insensitively', () => {
    expect(filterPlugins(plugins, { query: 'UPSTASH' }).map(plugin => plugin.name)).toEqual(['context7'])
    expect(filterPlugins(plugins, { query: 'skills' }).map(plugin => plugin.name)).toEqual(['example-external'])
    expect(filterPlugins(plugins, { query: 'version-specific' }).map(plugin => plugin.name)).toEqual(['context7'])
  })

  it('combines category, component, and source filters', () => {
    expect(filterPlugins(plugins, { category: 'documentation', component: 'mcp', source: 'built-in' }))
      .toEqual([plugins[0]])
    expect(filterPlugins(plugins, { source: 'external' })).toEqual([plugins[1]])
  })

  it('derives stable filter options from registry data', () => {
    expect(availableFilters(plugins)).toEqual({
      categories: ['development', 'documentation'],
      components: ['mcp', 'skills'],
    })
  })
})

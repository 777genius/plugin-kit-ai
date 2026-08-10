import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { pluginCommands } from '../utils/commands'
import { parseRegistryIndex } from '../utils/registry'

const fixture = JSON.parse(readFileSync(fileURLToPath(new URL('./fixtures/registry.valid.json', import.meta.url)), 'utf8')) as unknown
const plugins = parseRegistryIndex(fixture).plugins

describe('command generation', () => {
  it('generates the exact built-in lifecycle commands', () => {
    expect(pluginCommands(plugins[0]!, 'cursor')).toEqual({
      add: 'npx universal-agent-plugins add context7 --target cursor',
      update: 'npx universal-agent-plugins update context7 --target cursor',
      remove: 'npx universal-agent-plugins remove context7 --target cursor',
    })
  })

  it('keeps the exact pinned source for external add and the installed name thereafter', () => {
    const source = 'example/plugins@0123456789abcdef0123456789abcdef01234567//plugins/example'
    expect(pluginCommands(plugins[1]!, 'copilot')).toEqual({
      add: `npx universal-agent-plugins add ${source} --target copilot`,
      update: 'npx universal-agent-plugins update example-external --target copilot',
      remove: 'npx universal-agent-plugins remove example-external --target copilot',
    })
  })
})

import type { RegistryPlugin } from '../types/registry'

export function pluginCommands(plugin: RegistryPlugin, target: string) {
  return {
    add: `npx universal-agent-plugins add ${plugin.install_source} --target ${target}`,
    update: `npx universal-agent-plugins update ${plugin.name} --target ${target}`,
    remove: `npx universal-agent-plugins remove ${plugin.name} --target ${target}`,
  }
}

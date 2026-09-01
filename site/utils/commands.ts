import type { RegistryPlugin } from '../types/registry'

export function pluginCommands(plugin: RegistryPlugin, targets?: string | readonly string[]) {
  const values = (targets === undefined ? [] : Array.isArray(targets) ? targets : [targets])
    .map(target => target.trim())
    .filter((target, index, all) => target && all.indexOf(target) === index)
  if (targets !== undefined && !values.length) throw new Error('At least one target is required')
  const targetFlag = values.length ? ` --target ${values.join(',')}` : ''
  return {
    add: `npx universal-agent-plugins add ${plugin.install_source}${targetFlag}`,
    update: `npx universal-agent-plugins update ${plugin.name}${targetFlag}`,
    repair: `npx universal-agent-plugins repair ${plugin.name}${targetFlag}`,
    switch: `npx universal-agent-plugins switch ${plugin.name} --to <distribution-id>`,
    remove: `npx universal-agent-plugins remove ${plugin.name}${targetFlag}`,
  }
}

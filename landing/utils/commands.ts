import type { RegistryPlugin } from '../types/registry';

export function pluginCommands(plugin: RegistryPlugin, targets?: string | readonly string[]) {
  const values = (targets === undefined ? [] : Array.isArray(targets) ? targets : [targets])
    .map((target) => target.trim())
    .filter((target, index, all) => target && all.indexOf(target) === index);
  if (targets !== undefined && !values.length) throw new Error('At least one target is required');
  const targetFlag = values.length ? ` --target ${values.join(',')}` : '';
  return {
    add: `agentplugins add ${plugin.install_source}${targetFlag}`,
    update: `agentplugins update ${plugin.name}${targetFlag}`,
    repair: `agentplugins repair ${plugin.name}${targetFlag}`,
    switch: `agentplugins switch ${plugin.name} --to <distribution-id>`,
    remove: `agentplugins remove ${plugin.name}${targetFlag}`,
  };
}

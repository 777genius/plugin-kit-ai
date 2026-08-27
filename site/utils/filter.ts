import type { RegistryPlugin } from '../types/registry'

export interface CatalogFilters {
  query?: string
  category?: string
  component?: RegistryPlugin['components'][number]
  source?: 'all' | 'upstream' | 'community_bridge' | 'community' | 'direct'
  trust?: 'all' | 'reviewed' | 'conformant_unreviewed'
  client?: 'all' | RegistryPlugin['client_support']['clients'][number]
  authentication?: 'all' | 'none' | 'required_or_unknown'
  owner?: string
}

export function filterPlugins(plugins: RegistryPlugin[], filters: CatalogFilters): RegistryPlugin[] {
  const query = filters.query?.trim().toLocaleLowerCase() ?? ''
  const matches = plugins.filter((plugin) => {
    const searchable = [
      plugin.name,
      plugin.display_name,
      plugin.description,
      plugin.author.name,
      ...plugin.categories,
      ...plugin.keywords,
      ...plugin.components,
    ].join(' ').toLocaleLowerCase()
    return (!query || searchable.includes(query))
      && (!filters.category || plugin.categories.includes(filters.category))
      && (!filters.component || plugin.components.includes(filters.component))
      && (!filters.source || filters.source === 'all'
        || plugin.distributions.find(item => item.id === plugin.default_distribution)?.kind === filters.source)
      && (!filters.trust || filters.trust === 'all' || (plugin.trust_state ?? 'reviewed') === filters.trust)
      && (!filters.client || filters.client === 'all' || plugin.client_support.clients.includes(filters.client))
      && (!filters.authentication || filters.authentication === 'all'
        || (filters.authentication === 'none' ? plugin.authentication === 'none' : plugin.authentication !== 'none'))
      && (!filters.owner || plugin.author.name.toLocaleLowerCase() === filters.owner.toLocaleLowerCase())
  })
  return matches.sort((left, right) => {
    const score = (plugin: RegistryPlugin) => {
      const reviewed = (plugin.trust_state ?? 'reviewed') === 'reviewed'
      const name = plugin.name.toLocaleLowerCase()
      if (query && name === query) return reviewed ? 0 : 2
      if (query && name.startsWith(query)) return reviewed ? 1 : 3
      return reviewed ? 4 : 5
    }
    return score(left) - score(right)
      || ((right.discovery?.stars ?? 0) - (left.discovery?.stars ?? 0))
      || left.install_source.localeCompare(right.install_source)
  })
}

export function availableFilters(plugins: RegistryPlugin[]) {
  const categories = [...new Set(plugins.flatMap(plugin => plugin.categories))].sort()
  const components = [...new Set(plugins.flatMap(plugin => plugin.components))].sort()
  const owners = [...new Set(plugins.map(plugin => plugin.author.name))].sort((left, right) => left.localeCompare(right))
  return { categories, components, owners }
}

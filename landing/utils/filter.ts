import type { RegistryPlugin } from '../types/registry';

export interface CatalogFilters {
  query?: string;
  category?: string;
  component?: RegistryPlugin['components'][number];
  source?: 'all' | 'upstream' | 'community_bridge' | 'community' | 'direct';
  trust?: 'all' | 'reviewed' | 'conformant_unreviewed';
  client?: 'all' | RegistryPlugin['client_support']['clients'][number];
  authentication?: 'all' | 'none' | 'required_or_unknown';
  owner?: string;
}

/** Keep metadata-only Discovery records available by direct URL, not in the install catalog. */
export function catalogVisiblePlugins(plugins: RegistryPlugin[]): RegistryPlugin[] {
  return plugins.filter(
    (plugin) => plugin.trust_state !== 'conformant_unreviewed' || plugin.installable,
  );
}

function normalized(value: string): string {
  return value.trim().toLowerCase();
}

function sourceOwner(plugin: RegistryPlugin): string {
  return plugin.source.repository.split('/', 1)[0] ?? plugin.source.repository;
}

function compareText(left: string, right: string): number {
  return left < right ? -1 : left > right ? 1 : 0;
}

function compareDiscoveryPackages(left: RegistryPlugin, right: RegistryPlugin): number {
  const leftDepth = left.source.path ? left.source.path.split('/').length : 0;
  const rightDepth = right.source.path ? right.source.path.split('/').length : 0;
  return (
    leftDepth - rightDepth ||
    compareText(normalized(left.display_name), normalized(right.display_name))
  );
}

/** Stars belong to repositories, so popular monorepos are interleaved. */
function diversifiedDiscovery(plugins: RegistryPlugin[]): RegistryPlugin[] {
  const groups = new Map<string, RegistryPlugin[]>();
  for (const plugin of plugins) {
    const group = groups.get(plugin.source.repository) ?? [];
    group.push(plugin);
    groups.set(plugin.source.repository, group);
  }
  const repositories = [...groups.entries()].sort(
    ([leftRepository, left], [rightRepository, right]) =>
      (right[0]?.discovery?.stars ?? 0) - (left[0]?.discovery?.stars ?? 0) ||
      compareText(leftRepository, rightRepository),
  );
  repositories.forEach(([, group]) => group.sort(compareDiscoveryPackages));
  const ranked: RegistryPlugin[] = [];
  for (let index = 0; ranked.length < plugins.length; index += 1) {
    for (const [, group] of repositories) {
      if (group[index]) ranked.push(group[index]);
    }
  }
  return ranked;
}

function matchQuality(value: string, query: string): number | undefined {
  const candidate = normalized(value);
  if (candidate === query) return 0;
  if (candidate.startsWith(query)) return 1;
  if (candidate.split(/[^\p{L}\p{N}]+/u).some((part) => part.startsWith(query))) return 2;
  if (candidate.includes(query)) return 3;
  return undefined;
}

function textRelevance(plugin: RegistryPlugin, query: string): number {
  if (!query) return 0;
  const fields: Array<[number, string[]]> = [
    [0, [plugin.name, plugin.display_name]],
    [4, [...plugin.keywords, ...plugin.categories, ...plugin.components]],
    [8, [sourceOwner(plugin), plugin.source.repository]],
    [12, [plugin.author.name]],
    [16, [plugin.description]],
  ];
  return Math.min(
    ...fields.flatMap(([weight, values]) =>
      values
        .map((value) => matchQuality(value, query))
        .filter((quality): quality is number => quality !== undefined)
        .map((quality) => weight + quality),
    ),
    Number.MAX_SAFE_INTEGER,
  );
}

export function filterPlugins(
  plugins: RegistryPlugin[],
  filters: CatalogFilters,
): RegistryPlugin[] {
  const query = normalized(filters.query ?? '');
  const matches = plugins.filter((plugin) => {
    const searchable = [
      plugin.name,
      plugin.display_name,
      plugin.description,
      plugin.author.name,
      plugin.source.repository,
      ...plugin.categories,
      ...plugin.keywords,
      ...plugin.components,
    ];
    return (
      (!query || searchable.some((value) => normalized(value).includes(query))) &&
      (!filters.category || plugin.categories.includes(filters.category)) &&
      (!filters.component || plugin.components.includes(filters.component)) &&
      (!filters.source ||
        filters.source === 'all' ||
        plugin.distributions.find((item) => item.id === plugin.default_distribution)?.kind ===
          filters.source) &&
      (!filters.trust ||
        filters.trust === 'all' ||
        (plugin.trust_state ?? 'reviewed') === filters.trust) &&
      (!filters.client ||
        filters.client === 'all' ||
        plugin.client_support.clients.includes(filters.client)) &&
      (!filters.authentication ||
        filters.authentication === 'all' ||
        (filters.authentication === 'none'
          ? plugin.authentication === 'none'
          : plugin.authentication !== 'none')) &&
      (!filters.owner || normalized(sourceOwner(plugin)) === normalized(filters.owner))
    );
  });
  if (!query) {
    const reviewed = matches
      .filter((plugin) => (plugin.trust_state ?? 'reviewed') === 'reviewed')
      .sort((left, right) =>
        compareText(normalized(left.display_name), normalized(right.display_name)),
      );
    const discovered = diversifiedDiscovery(
      matches.filter((plugin) => plugin.trust_state === 'conformant_unreviewed'),
    );
    return [...reviewed, ...discovered];
  }
  return matches.sort((left, right) => {
    const score = (plugin: RegistryPlugin) => {
      const reviewed = (plugin.trust_state ?? 'reviewed') === 'reviewed';
      const name = normalized(plugin.name);
      if (query && reviewed && name === query) return 0;
      if (query && reviewed && name.startsWith(query)) return 1;
      if (query && !reviewed && name === query) return 2;
      return 3;
    };
    return (
      score(left) - score(right) ||
      textRelevance(left, query) - textRelevance(right, query) ||
      (right.discovery?.stars ?? 0) - (left.discovery?.stars ?? 0) ||
      compareText(left.install_source, right.install_source)
    );
  });
}

export function availableFilters(plugins: RegistryPlugin[]) {
  const categories = [...new Set(plugins.flatMap((plugin) => plugin.categories))].sort();
  const components = [...new Set(plugins.flatMap((plugin) => plugin.components))].sort();
  const owners = [...new Set(plugins.map(sourceOwner))].sort(compareText);
  return { categories, components, owners };
}

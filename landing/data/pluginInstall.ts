import { contentByLocale } from '~/data/content';
import type {
  PluginInstallCommandSet,
  PluginInstallScope,
  PluginInstallSpec,
  PluginInstallTargetId,
  PluginManageCommands,
  PluginResolvedInstallLane,
  PluginResolvedInstallSpec,
  PluginTargetInstallLane,
} from '~/types/content';

const targetDefinitions: Record<PluginInstallTargetId, PluginTargetInstallLane> = {
  claude: {
    targetId: 'claude',
    badgeLabel: 'Claude',
    scope: 'user',
    installPath: 'Claude user plugins',
    followUp: 'reload',
    vendorDocsHref: 'https://code.claude.com/docs/en/discover-plugins',
  },
  codex: {
    targetId: 'codex',
    badgeLabel: 'Codex',
    scope: 'user',
    installPath: 'Codex local plugin marketplace',
    followUp: 'activation',
    vendorDocsHref: 'https://developers.openai.com/codex/plugins',
  },
  gemini: {
    targetId: 'gemini',
    badgeLabel: 'Gemini',
    scope: 'user',
    installPath: '~/.gemini/extensions',
    followUp: 'enable',
    vendorDocsHref: 'https://geminicli.com/docs/extensions/reference/',
  },
  opencode: {
    targetId: 'opencode',
    badgeLabel: 'OpenCode',
    scope: 'project',
    installPath: 'opencode.json and .opencode/',
    projectRootRequired: true,
    followUp: 'none',
    vendorDocsHref: 'https://opencode.ai/docs/plugins/',
  },
  cursor: {
    targetId: 'cursor',
    badgeLabel: 'Cursor',
    scope: 'project',
    installPath: '.cursor/mcp.json',
    projectRootRequired: true,
    followUp: 'none',
    vendorDocsHref: 'https://docs.cursor.com/context/mcp',
  },
};

const defaultTargetOrder: PluginInstallTargetId[] = [
  'claude',
  'codex',
  'gemini',
  'opencode',
  'cursor',
];

const defaultSupportedTargets = defaultTargetOrder.map((targetId) => targetDefinitions[targetId]);

const catalogPluginSources = {
  context7: 'github:777genius/universal-plugins-for-ai-agents//plugins/context7',
  gitlab: 'github:777genius/universal-plugins-for-ai-agents//plugins/gitlab',
  github: 'github:777genius/universal-plugins-for-ai-agents//plugins/github',
  firebase: 'github:777genius/universal-plugins-for-ai-agents//plugins/firebase',
  linear: 'github:777genius/universal-plugins-for-ai-agents//plugins/linear',
  cloudflare: 'github:777genius/universal-plugins-for-ai-agents//plugins/cloudflare',
  'cloudflare-docs': 'github:777genius/universal-plugins-for-ai-agents//plugins/cloudflare-docs',
  'cloudflare-observability':
    'github:777genius/universal-plugins-for-ai-agents//plugins/cloudflare-observability',
  'cloudflare-bindings':
    'github:777genius/universal-plugins-for-ai-agents//plugins/cloudflare-bindings',
  'cloudflare-radar': 'github:777genius/universal-plugins-for-ai-agents//plugins/cloudflare-radar',
  'hubspot-crm': 'github:777genius/universal-plugins-for-ai-agents//plugins/hubspot-crm',
  'hubspot-developer':
    'github:777genius/universal-plugins-for-ai-agents//plugins/hubspot-developer',
  heroku: 'github:777genius/universal-plugins-for-ai-agents//plugins/heroku',
  neon: 'github:777genius/universal-plugins-for-ai-agents//plugins/neon',
  'docker-hub': 'github:777genius/universal-plugins-for-ai-agents//plugins/docker-hub',
  atlassian: 'github:777genius/universal-plugins-for-ai-agents//plugins/atlassian',
  notion: 'github:777genius/universal-plugins-for-ai-agents//plugins/notion',
  stripe: 'github:777genius/universal-plugins-for-ai-agents//plugins/stripe',
  slack: 'github:777genius/universal-plugins-for-ai-agents//plugins/slack',
  vercel: 'github:777genius/universal-plugins-for-ai-agents//plugins/vercel',
  sentry: 'github:777genius/universal-plugins-for-ai-agents//plugins/sentry',
  supabase: 'github:777genius/universal-plugins-for-ai-agents//plugins/supabase',
  greptile: 'github:777genius/universal-plugins-for-ai-agents//plugins/greptile',
} satisfies Record<string, string>;

const catalogPluginSpecs = Object.fromEntries(
  Object.entries(catalogPluginSources).map(([slug, cliSource]) => [
    slug,
    {
      slug,
      cliSource,
      integrationName: slug,
      recommendedTargetOrder: defaultTargetOrder,
      supportedTargets: defaultSupportedTargets,
    } satisfies PluginInstallSpec,
  ]),
) as Record<string, PluginInstallSpec>;

const buildInstallCommand = (
  cliSource: string,
  targetIds: PluginInstallTargetId[],
  scope?: PluginInstallScope,
  includeTargets = true,
): string => {
  const args = ['plugin-kit-ai', 'add', cliSource];

  if (includeTargets) {
    for (const targetId of targetIds) {
      args.push('--target', targetId);
    }
  }

  if (scope === 'project') {
    args.push('--scope', 'project');
  }

  args.push('--dry-run=false');
  return args.join(' ');
};

const buildManageCommands = (integrationName: string): PluginManageCommands => ({
  update: `plugin-kit-ai update ${integrationName} --dry-run=false`,
  repair: `plugin-kit-ai repair ${integrationName} --dry-run=false`,
  remove: `plugin-kit-ai remove ${integrationName} --dry-run=false`,
});

const buildCommandSet = (
  cliSource: string,
  integrationName: string,
  lane: PluginTargetInstallLane,
): PluginInstallCommandSet => ({
  install: buildInstallCommand(cliSource, [lane.targetId], lane.scope, true),
  update: `plugin-kit-ai update ${integrationName} --dry-run=false`,
  repair: `plugin-kit-ai repair ${integrationName} --target ${lane.targetId} --dry-run=false`,
  remove: `plugin-kit-ai remove ${integrationName} --dry-run=false`,
});

const resolveSupportedTargets = (spec: PluginInstallSpec): PluginResolvedInstallLane[] =>
  spec.supportedTargets.map((lane) => ({
    ...lane,
    commands: buildCommandSet(spec.cliSource, spec.integrationName, lane),
  }));

const assertCatalogCoverage = () => {
  const catalogSlugs = contentByLocale.en.plugins.map((plugin) => plugin.slug || plugin.id);
  const missingSlugs = catalogSlugs.filter((slug) => !catalogPluginSpecs[slug]);

  if (missingSlugs.length > 0) {
    throw new Error(
      `landing plugin install registry is missing specs for: ${missingSlugs.join(', ')}`,
    );
  }
};

assertCatalogCoverage();

export const getPluginInstallSpec = (slug: string): PluginInstallSpec | undefined => {
  return catalogPluginSpecs[slug];
};

export const getResolvedPluginInstallSpec = (
  slug: string,
): PluginResolvedInstallSpec | undefined => {
  const spec = getPluginInstallSpec(slug);
  if (!spec) {
    return undefined;
  }

  return {
    slug: spec.slug,
    cliSource: spec.cliSource,
    integrationName: spec.integrationName,
    recommendedTargetOrder: spec.recommendedTargetOrder,
    supportedTargets: resolveSupportedTargets(spec),
    manageCommands: buildManageCommands(spec.integrationName),
  };
};

export const buildInstallCommandForSelection = (
  spec: PluginResolvedInstallSpec,
  selectedTargetIds: PluginInstallTargetId[],
): string => {
  const allTargetIds = spec.recommendedTargetOrder.filter((targetId) =>
    spec.supportedTargets.some((lane) => lane.targetId === targetId),
  );
  const normalizedTargetIds = selectedTargetIds.length > 0 ? selectedTargetIds : allTargetIds;
  const selectedLanes = normalizedTargetIds
    .map((targetId) => spec.supportedTargets.find((lane) => lane.targetId === targetId))
    .filter((lane): lane is NonNullable<typeof lane> => Boolean(lane));
  const includeTargets = normalizedTargetIds.length !== allTargetIds.length;
  const uniqueScopes = [...new Set(selectedLanes.map((lane) => lane.scope))];
  const scope = !includeTargets
    ? undefined
    : uniqueScopes.length === 1 && uniqueScopes[0] === 'project'
      ? 'project'
      : undefined;

  return buildInstallCommand(spec.cliSource, normalizedTargetIds, scope, includeTargets);
};

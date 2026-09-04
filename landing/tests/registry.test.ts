import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { describe, it } from 'node:test';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { pluginCommands } from '../utils/commands.ts';
import { discoveryPlugin } from '../utils/discovery.ts';
import { catalogVisiblePlugins, filterPlugins } from '../utils/filter.ts';
import { parseDirectoryData } from '../utils/registry.ts';
import type { DiscoveryRecord, DiscoverySnapshot } from '../types/discovery.ts';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const pointer = JSON.parse(
  readFileSync(resolve(root, 'public/registry/schemas/1/latest.json'), 'utf8'),
) as { snapshot_path: string };
const snapshot = JSON.parse(
  readFileSync(resolve(root, 'public/registry/schemas/1', pointer.snapshot_path), 'utf8'),
) as unknown;
const registry = parseDirectoryData(snapshot, 'published_snapshot');
const discoverySnapshot = {
  sequence: 1,
  generated_at: '2026-01-01T00:00:00Z',
  expires_at: '2027-01-01T00:00:00Z',
} as DiscoverySnapshot;
const discoveryRecord = {
  slug: 'discovery:example/plugin',
  name: 'example',
  description: 'Example package',
  owner: 'example',
  repository: 'example/plugin',
  package_path: '',
  revision: '1'.repeat(40),
  version: '1.0.0',
  license: 'Apache-2.0',
  schema_version: '1.0.0',
  components: { extensions: 0, mcp: 0, skills: 0 },
  mcp_transports: [],
  compatible_clients: [],
  authentication: 'not_required',
  status: 'conformant_unreviewed',
  runtime_reviewed: false,
  tree_digest: `sha256:${'2'.repeat(64)}`,
  manifest_digest: `sha256:${'3'.repeat(64)}`,
  stars: 1,
  repository_updated_at: '2026-01-01T00:00:00Z',
  reviewed_distribution_id: null,
  availability: 'available',
} satisfies DiscoveryRecord;

describe('unified registry landing', () => {
  it('loads the signed reviewed directory used by the site', () => {
    assert.ok(registry.plugins.length >= 20);
    assert.ok(registry.plugins.every((plugin) => plugin.trust_state !== 'conformant_unreviewed'));
  });

  it('omits --target for installed-agent detection', () => {
    const plugin = registry.plugins.find((item) => item.installable);
    assert.ok(plugin);
    assert.equal(pluginCommands(plugin).add.includes('--target'), false);
    assert.match(pluginCommands(plugin).add, /^agentplugins add /);
  });

  it('uses one comma-separated target flag for explicit multi-agent installation', () => {
    const plugin = registry.plugins.find(
      (item) =>
        item.client_support.clients.includes('codex') &&
        item.client_support.clients.includes('cursor'),
    );
    assert.ok(plugin);
    assert.match(pluginCommands(plugin, ['codex', 'cursor']).add, / --target codex,cursor$/);
  });

  it('keeps reviewed results ahead of automatic discovery', () => {
    const reviewed = registry.plugins[0]!;
    const discovered = {
      ...reviewed,
      name: 'discovered-example',
      display_name: 'Discovered example',
      install_source: 'discovery:example/plugins//plugin',
      trust_state: 'conformant_unreviewed' as const,
      discovery: {
        sequence: 1,
        generated_at: '2026-01-01T00:00:00Z',
        expires_at: '2027-01-01T00:00:00Z',
        repository_updated_at: '2026-01-01T00:00:00Z',
        stars: 100_000,
        schema_version: '1.0.0' as const,
        manifest_digest: reviewed.source.manifest_sha256,
        tree_digest: reviewed.source.tree_sha256,
        mcp_transports: [],
        availability: 'available' as const,
      },
    };
    assert.equal(filterPlugins([discovered, reviewed], {})[0]?.name, reviewed.name);
  });

  it('keeps metadata-only discovery records out of the install catalog', () => {
    const metadataOnly = discoveryPlugin(discoveryRecord, discoverySnapshot);
    const portable = discoveryPlugin(
      {
        ...discoveryRecord,
        slug: 'discovery:example/portable',
        name: 'portable',
        components: { extensions: 0, mcp: 0, skills: 1 },
        compatible_clients: ['codex'],
      },
      discoverySnapshot,
    );

    assert.equal(metadataOnly.installable, false);
    assert.equal(metadataOnly.distributions[0]?.selectable, false);
    assert.equal(metadataOnly.distributions[0]?.releases[0]?.selectable, false);
    assert.equal(portable.installable, true);
    assert.deepEqual(catalogVisiblePlugins([metadataOnly, portable]), [portable]);
  });

  it('keeps every non-installable Discovery state out of the install catalog', () => {
    const unavailable = discoveryPlugin(
      {
        ...discoveryRecord,
        components: { extensions: 0, mcp: 1, skills: 0 },
        compatible_clients: ['codex'],
        availability: 'unavailable',
      },
      discoverySnapshot,
    );
    const unsupported = discoveryPlugin(
      {
        ...discoveryRecord,
        slug: 'discovery:example/unsupported',
        components: { extensions: 1, mcp: 0, skills: 0 },
      },
      discoverySnapshot,
    );
    const skillOnly = discoveryPlugin(
      {
        ...discoveryRecord,
        slug: 'discovery:example/skill-only',
        name: 'skill-only',
        components: { extensions: 0, mcp: 0, skills: 1 },
        compatible_clients: ['codex'],
      },
      discoverySnapshot,
    );

    assert.equal(unavailable.installable, false);
    assert.equal(unsupported.installable, false);
    assert.equal(skillOnly.installable, true);
    assert.deepEqual(catalogVisiblePlugins([unavailable, unsupported, skillOnly]), [skillOnly]);
  });
});

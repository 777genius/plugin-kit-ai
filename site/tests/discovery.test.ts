import assert from 'node:assert/strict'
import { createHash, generateKeyPairSync, sign } from 'node:crypto'
import { describe, it } from 'node:test'
import type { DiscoveryRecord } from '../types/discovery.ts'
import {
  discoveryPlugin,
  loadDiscovery,
  verifyDiscovery,
  type CachedDiscovery,
  type DiscoveryCache,
} from '../utils/discovery.ts'
import { pluginCommands } from '../utils/commands.ts'

const now = new Date('2026-08-27T12:00:00Z')
const encoder = new TextEncoder()

function canonical(value: unknown) {
  const sort = (item: unknown): unknown => Array.isArray(item)
    ? item.map(sort)
    : item && typeof item === 'object'
      ? Object.fromEntries(Object.entries(item).sort(([left], [right]) => left < right ? -1 : left > right ? 1 : 0).map(([key, child]) => [key, sort(child)]))
      : item
  return encoder.encode(`${JSON.stringify(sort(value))}\n`)
}

function digest(bytes: Uint8Array) {
  return `sha256:${createHash('sha256').update(bytes).digest('hex')}`
}

function message(snapshot: Uint8Array) {
  const prefix = encoder.encode('UAP-DISCOVERY-INDEX-ED25519-V1\0')
  const result = new Uint8Array(prefix.length + 8 + snapshot.length)
  result.set(prefix)
  new DataView(result.buffer).setBigUint64(prefix.length, BigInt(snapshot.length))
  result.set(snapshot, prefix.length + 8)
  return result
}

function fixture(sequence = 7) {
  const record: DiscoveryRecord = {
    slug: 'discovery:example/portable//agent-plugin',
    name: 'portable-demo',
    description: 'Portable test package',
    owner: 'example',
    repository: 'example/portable',
    package_path: 'agent-plugin',
    revision: 'a'.repeat(40),
    version: '1.2.3',
    license: 'Apache-2.0',
    schema_version: '1.0.0',
    components: { extensions: 0, mcp: 1, skills: 1 },
    mcp_transports: ['stdio'],
    compatible_clients: ['codex', 'cursor'],
    authentication: 'unknown',
    status: 'conformant_unreviewed',
    runtime_reviewed: false,
    tree_digest: `sha256:${'1'.repeat(64)}`,
    manifest_digest: `sha256:${'2'.repeat(64)}`,
    stars: 412,
    repository_updated_at: '2026-08-27T10:00:00Z',
    reviewed_distribution_id: null,
    availability: 'available',
  }
  const searchRecord = structuredClone(record)
  const fullRecord = { ...record, author: { name: 'Example' }, first_seen: '2026-08-27T10:00:00Z', last_seen: '2026-08-27T11:00:00Z' }
  const search = { search_schema_version: 1, sequence, generated_at: '2026-08-27T11:00:00Z', records: [searchRecord] }
  const searchBytes = canonical(search)
  const snapshot = {
    discovery_schema_version: 1,
    sequence,
    publication_id: `test-${sequence}`,
    source_commit: 'b'.repeat(40),
    generated_at: '2026-08-27T11:00:00Z',
    expires_at: '2026-08-30T11:00:00Z',
    complete: true,
    query_manifest_digest: `sha256:${'3'.repeat(64)}`,
    partitions: [{ query: 'path:plugin.json', size_min: 0, size_max: 1023, total_count: 1 }],
    search_projection: { path: `search/${String(sequence).padStart(20, '0')}.json`, digest: digest(searchBytes), record_count: 1 },
    records: [fullRecord],
  }
  const snapshotBytes = canonical(snapshot)
  const { privateKey, publicKey } = generateKeyPairSync('ed25519')
  const rawPublicKey = publicKey.export({ format: 'der', type: 'spki' }).subarray(-32)
  const envelope = {
    envelope_schema_version: 1,
    snapshot_schema_version: 1,
    sequence,
    key_id: 'test-discovery',
    algorithm: 'Ed25519',
    signature_domain: 'UAP-DISCOVERY-INDEX-ED25519-V1',
    snapshot_digest: digest(snapshotBytes),
    signature: sign(null, message(snapshotBytes), privateKey).toString('base64'),
  }
  const stem = String(sequence).padStart(20, '0')
  const pointer = {
    pointer_schema_version: 1,
    snapshot_schema_version: 1,
    sequence,
    snapshot_path: `snapshots/${stem}.json`,
    envelope_path: `snapshots/${stem}.envelope.json`,
    search_path: `search/${stem}.json`,
    fetch_contract: {
      max_redirects: 0,
      latest_max_bytes: 16 << 10,
      snapshot_max_bytes: 16 << 20,
      envelope_max_bytes: 16 << 10,
      search_max_bytes: 10 << 20,
      retry_attempts: 1,
    },
  }
  return {
    bytes: { pointer: canonical(pointer), snapshot: snapshotBytes, envelope: canonical(envelope), search: searchBytes },
    trust: { keyID: 'test-discovery', publicKeyBase64: rawPublicKey.toString('base64') },
    snapshot,
    record,
  }
}

class MemoryCache implements DiscoveryCache {
  value?: CachedDiscovery
  async load() { return this.value }
  async store(value: CachedDiscovery) { this.value = structuredClone(value) }
}

describe('signed public Discovery Index', () => {
  it('verifies exact bytes and maps one unreviewed package to a publisher-qualified command', async () => {
    const data = fixture()
    const bundle = await verifyDiscovery(data.bytes, data.trust, {}, now)
    const plugin = discoveryPlugin(bundle.search.records[0]!, bundle.snapshot)
    assert.equal(bundle.snapshot.sequence, 7)
    assert.equal(plugin.trust_state, 'conformant_unreviewed')
    assert.equal(plugin.source.revision, 'a'.repeat(40))
    assert.equal(pluginCommands(plugin, ['codex', 'cursor']).add, 'npx universal-agent-plugins add discovery:example/portable//agent-plugin --target codex,cursor')
  })

  it('rejects tampered projections, expired snapshots, and sequence equivocation', async () => {
    const data = fixture()
    const tampered = structuredClone(data.bytes)
    tampered.search[tampered.search.length - 2] ^= 1
    await assert.rejects(verifyDiscovery(tampered, data.trust, {}, now), /canonical|digest|JSON/)
    await assert.rejects(verifyDiscovery(data.bytes, data.trust, {}, new Date('2026-08-31T00:00:00Z')), /stale/)

    const cache = new MemoryCache()
    const origin = new URL('https://catalog.example/discovery/')
    const first = makeFetcher(origin, data.bytes)
    await loadDiscovery({ origin, trust: data.trust, cache, fetcher: first, now })
    const conflicting = structuredClone(data.bytes)
    conflicting.pointer = canonical({ ...JSON.parse(new TextDecoder().decode(data.bytes.pointer)), fetch_contract: { ...JSON.parse(new TextDecoder().decode(data.bytes.pointer)).fetch_contract, retry_attempts: 2 } })
    await assert.rejects(loadDiscovery({ origin, trust: data.trust, cache, fetcher: makeFetcher(origin, conflicting), now }), /equivocation/)
  })

  it('uses the last-known-good signed cache when the network is unavailable', async () => {
    const data = fixture()
    const cache = new MemoryCache()
    const origin = new URL('https://catalog.example/discovery/')
    const remote = await loadDiscovery({ origin, trust: data.trust, cache, fetcher: makeFetcher(origin, data.bytes), now })
    assert.equal(remote.source, 'remote')
    const cached = await loadDiscovery({ origin, trust: data.trust, cache, fetcher: async () => { throw new Error('offline') }, now })
    assert.equal(cached.source, 'cache')
    assert.equal(cached.snapshot.sequence, 7)
  })

  it('keeps the last-known-good cache when the pointer rolls back', async () => {
    const current = fixture(7)
    const older = fixture(6)
    const cache = new MemoryCache()
    const origin = new URL('https://catalog.example/discovery/')
    await loadDiscovery({ origin, trust: current.trust, cache, fetcher: makeFetcher(origin, current.bytes, 7), now })
    const rolledBack = await loadDiscovery({
      origin,
      trust: current.trust,
      cache,
      fetcher: makeFetcher(origin, older.bytes, 6),
      now,
    })
    assert.equal(rolledBack.source, 'cache')
    assert.equal(rolledBack.snapshot.sequence, 7)
  })
})

function makeFetcher(origin: URL, bytes: ReturnType<typeof fixture>['bytes'], sequence = 7): typeof fetch {
  const stem = String(sequence).padStart(20, '0')
  const values = new Map([
    [new URL('latest.json', origin).href, bytes.pointer],
    [new URL(`snapshots/${stem}.json`, origin).href, bytes.snapshot],
    [new URL(`snapshots/${stem}.envelope.json`, origin).href, bytes.envelope],
    [new URL(`search/${stem}.json`, origin).href, bytes.search],
  ])
  return (async (input) => {
    const url = String(input)
    const body = values.get(url)
    return body
      ? new Response(body, { status: 200, headers: { 'content-type': 'application/json', etag: '"fixture"' } })
      : new Response('missing', { status: 404 })
  }) as typeof fetch
}

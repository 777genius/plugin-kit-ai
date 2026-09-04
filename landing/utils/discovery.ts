import type {
  DiscoveryBundle,
  DiscoveryEnvelope,
  DiscoveryPointer,
  DiscoveryRecord,
  DiscoverySearch,
  DiscoverySnapshot,
} from '../types/discovery';
import type { ClientID, ComponentID, RegistryPlugin } from '../types/registry';

const signatureDomain = 'UAP-DISCOVERY-INDEX-ED25519-V1\0';
const digestPattern = /^sha256:[0-9a-f]{64}$/;
const revisionPattern = /^[0-9a-f]{40}$/;
const timestampPattern = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$/;
const repositoryPattern = /^[a-z0-9][a-z0-9-]*\/[a-z0-9][a-z0-9._-]*$/;
const packagePathPattern = /^(?:|[A-Za-z0-9._-]+(?:\/[A-Za-z0-9._-]+)*)$/;
// Unreviewed Discovery metadata cannot assert the registered app binding that
// ChatGPT requires. Reviewed Directory products add ChatGPT independently.
const clientOrder: ClientID[] = ['codex', 'cursor', 'copilot', 'vscode', 'kiro'];
const transportOrder = ['sse', 'stdio', 'streamable-http'];
const encoder = new TextEncoder();
const decoder = new TextDecoder('utf-8', { fatal: true });

type ArtifactName = keyof DiscoveryBundle['bytes'];

export interface DiscoveryTrust {
  keyID: string;
  publicKeyBase64: string;
}

export interface CachedDiscovery {
  bytes: Record<ArtifactName, Uint8Array>;
  etags: DiscoveryBundle['etags'];
}

export interface DiscoveryCache {
  load(): Promise<CachedDiscovery | undefined>;
  store(value: CachedDiscovery): Promise<void>;
}

export interface DiscoveryLoadOptions {
  origin: URL;
  trust: DiscoveryTrust;
  cache?: DiscoveryCache;
  fetcher?: typeof fetch;
  now?: Date;
}

export class DiscoveryEquivocationError extends Error {}

export class BrowserDiscoveryCache implements DiscoveryCache {
  readonly cacheName = 'uap-discovery-v1';
  readonly entry: URL;

  constructor(origin: URL) {
    this.entry = new URL('.browser-lkg.json', origin);
  }

  async load(): Promise<CachedDiscovery | undefined> {
    if (!globalThis.caches) return undefined;
    const response = await (await caches.open(this.cacheName)).match(this.entry);
    if (!response?.ok) return undefined;
    const value = (await response.json()) as {
      schema: number;
      bytes: Record<ArtifactName, string>;
      etags: DiscoveryBundle['etags'];
    };
    if (value.schema !== 1) return undefined;
    return {
      bytes: {
        pointer: decodeBase64(value.bytes.pointer),
        envelope: decodeBase64(value.bytes.envelope),
        snapshot: decodeBase64(value.bytes.snapshot),
        search: decodeBase64(value.bytes.search),
      },
      etags: value.etags ?? {},
    };
  }

  async store(value: CachedDiscovery): Promise<void> {
    if (!globalThis.caches) return;
    const body = JSON.stringify({
      schema: 1,
      bytes: Object.fromEntries(
        Object.entries(value.bytes).map(([key, bytes]) => [key, encodeBase64(bytes)]),
      ),
      etags: value.etags,
    });
    await (
      await caches.open(this.cacheName)
    ).put(
      this.entry,
      new Response(body, {
        headers: { 'content-type': 'application/json' },
      }),
    );
  }
}

export async function loadDiscovery(options: DiscoveryLoadOptions): Promise<DiscoveryBundle> {
  const fetcher = options.fetcher ?? fetch;
  const now = options.now ?? new Date();
  const cachedRaw = await options.cache?.load().catch(() => undefined);
  const cached = cachedRaw
    ? await verifyDiscovery(cachedRaw.bytes, options.trust, cachedRaw.etags, now).catch(
        () => undefined,
      )
    : undefined;
  const pointerResponse = await fetchArtifact(
    new URL('latest.json', options.origin),
    16 << 10,
    fetcher,
    cachedRaw?.etags.pointer,
  ).catch((error: unknown) => ({ error }));

  if ('error' in pointerResponse || pointerResponse.notModified) {
    if (cached) return { ...cached, source: 'cache' };
    throw 'error' in pointerResponse
      ? pointerResponse.error
      : new Error('Discovery returned 304 without a valid cache');
  }

  try {
    const pointer = parsePointer(pointerResponse.bytes);
    if (cached && pointer.sequence < cached.snapshot.sequence)
      return { ...cached, source: 'cache' };
    if (cached && pointer.sequence === cached.snapshot.sequence) {
      if (!bytesEqual(pointerResponse.bytes, cached.bytes.pointer))
        throw new DiscoveryEquivocationError('Discovery sequence equivocation detected');
      return { ...cached, source: 'cache' };
    }

    const artifactSpecs: Array<[Exclude<ArtifactName, 'pointer'>, string, number]> = [
      ['snapshot', pointer.snapshot_path, pointer.fetch_contract.snapshot_max_bytes],
      ['envelope', pointer.envelope_path, pointer.fetch_contract.envelope_max_bytes],
      ['search', pointer.search_path, pointer.fetch_contract.search_max_bytes],
    ];
    const loaded = await Promise.all(
      artifactSpecs.map(async ([name, path, maximum]) => {
        const response = await fetchArtifact(
          new URL(path, options.origin),
          maximum,
          fetcher,
          undefined,
        );
        if (response.notModified) throw new Error(`Discovery ${name} returned an unexpected 304`);
        return [name, response] as const;
      }),
    );
    const responses = Object.fromEntries(loaded) as Record<
      Exclude<ArtifactName, 'pointer'>,
      ArtifactResponse
    >;
    const bytes = {
      pointer: pointerResponse.bytes,
      snapshot: responses.snapshot.bytes,
      envelope: responses.envelope.bytes,
      search: responses.search.bytes,
    };
    const etags = {
      pointer: pointerResponse.etag,
      snapshot: responses.snapshot.etag,
      envelope: responses.envelope.etag,
      search: responses.search.etag,
    };
    const verified = await verifyDiscovery(bytes, options.trust, etags, now);
    await options.cache?.store({ bytes, etags }).catch(() => undefined);
    return { ...verified, source: 'remote' };
  } catch (error) {
    if (error instanceof DiscoveryEquivocationError) throw error;
    if (cached) return { ...cached, source: 'cache' };
    throw error;
  }
}

export async function verifyDiscovery(
  bytes: Record<ArtifactName, Uint8Array>,
  trust: DiscoveryTrust,
  etags: DiscoveryBundle['etags'] = {},
  now = new Date(),
): Promise<DiscoveryBundle> {
  const pointer = parsePointer(bytes.pointer);
  const envelope = parseCanonical<DiscoveryEnvelope>(bytes.envelope, 'envelope');
  const snapshot = parseCanonical<DiscoverySnapshot>(bytes.snapshot, 'snapshot');
  const search = parseCanonical<DiscoverySearch>(bytes.search, 'search');
  assertEnvelope(envelope, trust);
  assertSnapshot(snapshot);
  assertSearch(search);
  if (
    pointer.sequence !== snapshot.sequence ||
    envelope.sequence !== snapshot.sequence ||
    search.sequence !== snapshot.sequence
  ) {
    throw new Error('Discovery sequence mismatch');
  }
  if (
    pointer.search_path !== snapshot.search_projection.path ||
    search.generated_at !== snapshot.generated_at
  ) {
    throw new Error('Discovery projection mismatch');
  }
  if (
    search.records.length !== snapshot.records.length ||
    search.records.length !== snapshot.search_projection.record_count
  ) {
    throw new Error('Discovery record count mismatch');
  }
  if (
    (await sha256(bytes.snapshot)) !== envelope.snapshot_digest ||
    (await sha256(bytes.search)) !== snapshot.search_projection.digest
  ) {
    throw new Error('Discovery digest mismatch');
  }
  const publicKey = decodeBase64(trust.publicKeyBase64);
  const signature = decodeBase64(envelope.signature);
  if (publicKey.length !== 32 || signature.length !== 64)
    throw new Error('Discovery key or signature has an invalid length');
  const key = await crypto.subtle.importKey('raw', publicKey, { name: 'Ed25519' }, false, [
    'verify',
  ]);
  if (!(await crypto.subtle.verify('Ed25519', key, signature, signatureMessage(bytes.snapshot)))) {
    throw new Error('Discovery signature is invalid');
  }
  for (let index = 0; index < snapshot.records.length; index += 1) {
    const {
      author: _author,
      first_seen: _firstSeen,
      last_seen: _lastSeen,
      ...compact
    } = snapshot.records[index]!;
    if (canonicalValue(compact) !== canonicalValue(search.records[index]))
      throw new Error('Discovery search record mismatch');
  }
  const generated = parseTimestamp(snapshot.generated_at);
  const expires = parseTimestamp(snapshot.expires_at);
  if (now.getTime() < generated.getTime())
    throw new Error('Discovery local clock is before generation time');
  if (now.getTime() >= expires.getTime()) throw new Error('Discovery snapshot is stale');
  return { pointer, envelope, snapshot, search, bytes, etags, source: 'cache' };
}

export function discoveryPlugin(
  record: DiscoveryRecord,
  snapshot: DiscoverySnapshot,
): RegistryPlugin {
  const components = (Object.entries(record.components) as Array<[ComponentID, number]>)
    .filter(([, count]) => count > 0)
    .map(([component]) => component);
  const targets = record.compatible_clients.map((client) => ({
    client,
    authentication: record.authentication,
    delivery: 'managed' as const,
    scopes: ['user'],
  }));
  const installable =
    record.availability === 'available' && components.length > 0 && targets.length > 0;
  const revision = record.revision;
  const source = {
    repository: record.repository,
    revision,
    path: record.package_path,
    manifest_sha256: record.manifest_digest,
    tree_sha256: record.tree_digest,
  };
  const release = {
    release_sequence: snapshot.sequence,
    source,
    version: record.version ?? 'unknown',
    targets,
    components,
    evidence: [],
    package_evidence: [],
    release_status: 'active' as const,
    selectable: installable,
    blocking_clients: [],
    materialized_clients: [],
    meets_minimum_capabilities: true,
  };
  return {
    name: record.name,
    display_name: record.name,
    version: record.version ?? 'unknown',
    description: record.description || `Agent Plugins 1.0 package from ${record.repository}`,
    author: record.author ?? { name: record.owner, url: `https://github.com/${record.owner}` },
    license: record.license ?? 'Not specified',
    categories: [],
    keywords: [record.owner, record.repository, ...record.mcp_transports],
    source,
    install_source: record.slug,
    built_in: false,
    installable,
    components,
    default_distribution: record.slug,
    declared_default_distribution: record.slug,
    distributions: [
      {
        id: record.slug,
        kind: 'direct',
        label: 'Unreviewed discovery source',
        publisher: record.owner,
        source,
        release_sequence: snapshot.sequence,
        version: release.version,
        compatible_clients: record.compatible_clients,
        evidence: [],
        package_evidence: [],
        status: record.availability === 'available' ? 'active' : 'suspended',
        release_status: 'active',
        selectable: installable,
        targets,
        components,
        releases: [release],
      },
    ],
    evidence: [],
    package_evidence: [],
    authentication: record.authentication === 'not_required' ? 'none' : 'unknown',
    client_support: {
      resolution: 'install_time',
      clients: record.compatible_clients,
      delivery: {},
      scopes: {},
      app_bindings: {},
    },
    trust_state: 'conformant_unreviewed',
    discovery: {
      sequence: snapshot.sequence,
      generated_at: snapshot.generated_at,
      expires_at: snapshot.expires_at,
      repository_updated_at: record.repository_updated_at,
      stars: record.stars,
      schema_version: record.schema_version,
      manifest_digest: record.manifest_digest,
      tree_digest: record.tree_digest,
      mcp_transports: record.mcp_transports,
      availability: record.availability,
      ...(record.reviewed_distribution_id
        ? { reviewed_distribution_id: record.reviewed_distribution_id }
        : {}),
    },
  };
}

interface ArtifactResponse {
  bytes: Uint8Array;
  etag?: string;
  notModified: boolean;
}

async function fetchArtifact(
  url: URL,
  maximum: number,
  fetcher: typeof fetch,
  etag?: string,
): Promise<ArtifactResponse> {
  const headers = new Headers({ accept: 'application/json' });
  if (etag) headers.set('if-none-match', etag);
  const response = await fetcher(url, {
    cache: 'no-cache',
    credentials: 'omit',
    headers,
    redirect: 'error',
  });
  if (response.status === 304) return { bytes: new Uint8Array(), etag, notModified: true };
  if (!response.ok || (response.url && new URL(response.url).origin !== url.origin))
    throw new Error(`Discovery request failed with HTTP ${response.status}`);
  const declared = Number(response.headers.get('content-length'));
  if (Number.isFinite(declared) && declared > maximum)
    throw new Error('Discovery response exceeds its size limit');
  const bytes = new Uint8Array(await response.arrayBuffer());
  if (!bytes.length || bytes.length > maximum)
    throw new Error('Discovery response exceeds its size limit');
  return { bytes, etag: response.headers.get('etag') ?? undefined, notModified: false };
}

function parsePointer(bytes: Uint8Array): DiscoveryPointer {
  const value = parseCanonical<DiscoveryPointer>(bytes, 'pointer');
  assertKeys(
    value,
    [
      'pointer_schema_version',
      'snapshot_schema_version',
      'sequence',
      'snapshot_path',
      'envelope_path',
      'search_path',
      'fetch_contract',
    ],
    'pointer',
  );
  assertKeys(
    value.fetch_contract,
    [
      'max_redirects',
      'latest_max_bytes',
      'snapshot_max_bytes',
      'envelope_max_bytes',
      'search_max_bytes',
      'retry_attempts',
    ],
    'fetch contract',
  );
  requirePublicSequence(value.sequence, 'pointer');
  const stem = String(value.sequence).padStart(20, '0');
  const contract = value.fetch_contract;
  if (
    value.pointer_schema_version !== 1 ||
    value.snapshot_schema_version !== 1 ||
    value.snapshot_path !== `snapshots/${stem}.json` ||
    value.envelope_path !== `snapshots/${stem}.envelope.json` ||
    value.search_path !== `search/${stem}.json` ||
    contract.max_redirects !== 0 ||
    contract.retry_attempts < 1 ||
    contract.retry_attempts > 3 ||
    contract.latest_max_bytes < 1 ||
    contract.latest_max_bytes > 16 << 10 ||
    contract.snapshot_max_bytes < 1 ||
    contract.snapshot_max_bytes > 16 << 20 ||
    contract.envelope_max_bytes < 1 ||
    contract.envelope_max_bytes > 16 << 10 ||
    contract.search_max_bytes < 1 ||
    contract.search_max_bytes > 10 << 20
  ) {
    throw new Error('Discovery pointer is invalid');
  }
  return value;
}

function assertEnvelope(value: DiscoveryEnvelope, trust: DiscoveryTrust) {
  assertKeys(
    value,
    [
      'envelope_schema_version',
      'snapshot_schema_version',
      'sequence',
      'key_id',
      'algorithm',
      'signature_domain',
      'snapshot_digest',
      'signature',
    ],
    'envelope',
  );
  requirePublicSequence(value.sequence, 'envelope');
  if (
    value.envelope_schema_version !== 1 ||
    value.snapshot_schema_version !== 1 ||
    value.key_id !== trust.keyID ||
    value.algorithm !== 'Ed25519' ||
    value.signature_domain !== 'UAP-DISCOVERY-INDEX-ED25519-V1' ||
    !digestPattern.test(value.snapshot_digest) ||
    !/^[A-Za-z0-9+/]{86}==$/.test(value.signature)
  ) {
    throw new Error('Discovery envelope is invalid');
  }
}

function assertSnapshot(value: DiscoverySnapshot) {
  assertKeys(
    value,
    [
      'discovery_schema_version',
      'sequence',
      'publication_id',
      'source_commit',
      'generated_at',
      'expires_at',
      'complete',
      'query_manifest_digest',
      'partitions',
      'search_projection',
      'records',
    ],
    'snapshot',
  );
  assertKeys(value.search_projection, ['path', 'digest', 'record_count'], 'search projection');
  requirePublicSequence(value.sequence, 'snapshot');
  if (
    value.discovery_schema_version !== 1 ||
    !value.complete ||
    !/^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/.test(value.publication_id) ||
    !revisionPattern.test(value.source_commit) ||
    !digestPattern.test(value.query_manifest_digest) ||
    value.search_projection.path !== `search/${String(value.sequence).padStart(20, '0')}.json` ||
    !digestPattern.test(value.search_projection.digest) ||
    value.search_projection.record_count !== value.records.length ||
    value.records.length > 10_000
  )
    throw new Error('Discovery snapshot is invalid');
  const generated = parseTimestamp(value.generated_at);
  const expires = parseTimestamp(value.expires_at);
  if (expires <= generated || expires.getTime() - generated.getTime() > 7 * 86_400_000)
    throw new Error('Discovery lifetime is invalid');
  value.partitions.forEach((partition) => {
    assertKeys(partition, ['query', 'size_min', 'size_max', 'total_count'], 'partition');
    if (
      !partition.query ||
      !integer(partition.size_min) ||
      !integer(partition.size_max) ||
      !integer(partition.total_count) ||
      partition.size_min < 0 ||
      partition.size_max < partition.size_min ||
      partition.total_count < 0 ||
      partition.total_count > 1000
    ) {
      throw new Error('Discovery partition is invalid');
    }
  });
  assertRecords(value.records, true, generated);
}

function assertSearch(value: DiscoverySearch) {
  assertKeys(value, ['search_schema_version', 'sequence', 'generated_at', 'records'], 'search');
  requirePublicSequence(value.sequence, 'search');
  if (value.search_schema_version !== 1 || value.records.length > 10_000) {
    throw new Error('Discovery search projection is invalid');
  }
  assertRecords(value.records, false, parseTimestamp(value.generated_at));
}

function assertRecords(records: DiscoveryRecord[], full: boolean, generated: Date) {
  const identities = new Set<string>();
  const slugs = new Set<string>();
  let prior = '';
  records.forEach((record) => {
    const commonFields = [
      'slug',
      'name',
      'description',
      'owner',
      'repository',
      'package_path',
      'revision',
      'version',
      'license',
      'schema_version',
      'components',
      'mcp_transports',
      'compatible_clients',
      'authentication',
      'status',
      'runtime_reviewed',
      'tree_digest',
      'manifest_digest',
      'stars',
      'repository_updated_at',
      'reviewed_distribution_id',
      'availability',
    ];
    assertKeys(
      record,
      full ? [...commonFields, 'author', 'first_seen', 'last_seen'] : commonFields,
      'record',
    );
    assertKeys(record.components, ['extensions', 'mcp', 'skills'], 'components');
    if (full && record.author) {
      const authorKeys = Object.keys(record.author);
      if (
        !authorKeys.includes('name') ||
        authorKeys.some((key) => !['name', 'email', 'url'].includes(key))
      )
        throw new Error('Discovery author is invalid');
    }
    const expectedSlug = `discovery:${record.repository}${record.package_path ? `//${record.package_path}` : ''}`;
    const identity = `${record.repository}\0${record.package_path.toLocaleLowerCase()}`;
    const order = `${record.repository}\0${record.package_path.toLocaleLowerCase()}\0${record.slug}`;
    if (
      !repositoryPattern.test(record.repository) ||
      record.owner !== record.repository.split('/')[0] ||
      !packagePathPattern.test(record.package_path) ||
      !revisionPattern.test(record.revision) ||
      record.slug !== expectedSlug ||
      identities.has(identity) ||
      slugs.has(record.slug) ||
      order < prior ||
      record.schema_version !== '1.0.0' ||
      record.status !== 'conformant_unreviewed' ||
      record.runtime_reviewed !== false ||
      !digestPattern.test(record.tree_digest) ||
      !digestPattern.test(record.manifest_digest) ||
      !integer(record.stars) ||
      record.stars < 0 ||
      (record.version !== null && typeof record.version !== 'string') ||
      (record.license !== null && typeof record.license !== 'string') ||
      (record.reviewed_distribution_id !== null &&
        typeof record.reviewed_distribution_id !== 'string') ||
      !['available', 'unavailable'].includes(record.availability) ||
      record.name.length < 1 ||
      [...record.name].length > 64 ||
      [...record.description].length > 500 ||
      !Object.values(record.components).every((count) => integer(count) && count >= 0) ||
      !orderedEnums(record.compatible_clients, clientOrder) ||
      !orderedEnums(record.mcp_transports, transportOrder) ||
      !['not_required', 'required', 'unknown'].includes(record.authentication)
    )
      throw new Error(`Discovery record ${record.slug} is invalid`);
    parseTimestamp(record.repository_updated_at);
    if (full) {
      const first = parseTimestamp(record.first_seen ?? '');
      const last = parseTimestamp(record.last_seen ?? '');
      if (first > last || last > generated || (record.author && !record.author.name.trim()))
        throw new Error('Discovery seen interval is invalid');
    } else if (
      record.author !== undefined ||
      record.first_seen !== undefined ||
      record.last_seen !== undefined
    ) {
      throw new Error('Discovery search contains snapshot-only metadata');
    }
    identities.add(identity);
    slugs.add(record.slug);
    prior = order;
  });
}

function parseCanonical<T>(bytes: Uint8Array, label: string): T {
  const text = decoder.decode(bytes);
  const value = JSON.parse(text) as T;
  if (canonicalValue(value) !== text) throw new Error(`Discovery ${label} is not canonical JSON`);
  return value;
}

function canonicalValue(value: unknown): string {
  validateCanonical(value);
  const sort = (item: unknown): unknown =>
    Array.isArray(item)
      ? item.map(sort)
      : item && typeof item === 'object'
        ? Object.fromEntries(
            Object.entries(item)
              .sort(([left], [right]) => (left < right ? -1 : left > right ? 1 : 0))
              .map(([key, child]) => [key, sort(child)]),
          )
        : item;
  return `${JSON.stringify(sort(value))}\n`;
}

function validateCanonical(value: unknown) {
  if (value === null || typeof value === 'boolean') return;
  if (typeof value === 'number') {
    if (!Number.isSafeInteger(value))
      throw new Error('Discovery JSON contains a non-integer number');
    return;
  }
  if (typeof value === 'string') {
    if (value !== value.normalize('NFC')) throw new Error('Discovery JSON contains non-NFC text');
    return;
  }
  if (Array.isArray(value)) return value.forEach(validateCanonical);
  if (!value || typeof value !== 'object')
    throw new Error('Discovery JSON contains an unsupported value');
  const folded = new Set<string>();
  Object.entries(value).forEach(([key, child]) => {
    if (key !== key.normalize('NFC') || folded.has(key.toLocaleLowerCase()))
      throw new Error('Discovery JSON contains colliding keys');
    folded.add(key.toLocaleLowerCase());
    validateCanonical(child);
  });
}

async function sha256(bytes: Uint8Array) {
  const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', Uint8Array.from(bytes)));
  return `sha256:${[...digest].map((byte) => byte.toString(16).padStart(2, '0')).join('')}`;
}

function signatureMessage(snapshot: Uint8Array) {
  const prefix = encoder.encode(signatureDomain);
  const result = new Uint8Array(prefix.length + 8 + snapshot.length);
  result.set(prefix);
  new DataView(result.buffer).setBigUint64(prefix.length, BigInt(snapshot.length));
  result.set(snapshot, prefix.length + 8);
  return result;
}

function parseTimestamp(value: string) {
  if (!timestampPattern.test(value))
    throw new Error('Discovery timestamp must use second-precision UTC');
  const parsed = new Date(value);
  if (!Number.isFinite(parsed.getTime())) throw new Error('Discovery timestamp is invalid');
  return parsed;
}

function orderedEnums(values: readonly string[], allowed: readonly string[]) {
  if (!Array.isArray(values)) return false;
  let previous = -1;
  for (const value of values) {
    const position = allowed.indexOf(value);
    if (position <= previous) return false;
    previous = position;
  }
  return true;
}

function integer(value: number) {
  return Number.isSafeInteger(value);
}

function requirePublicSequence(value: number, label: string) {
  if (!Number.isSafeInteger(value) || value < 1)
    throw new Error(`Discovery ${label} sequence is invalid`);
}

function assertKeys(value: object, expected: string[], label: string) {
  const actual = Object.keys(value).sort();
  const wanted = [...expected].sort();
  if (actual.length !== wanted.length || actual.some((key, index) => key !== wanted[index])) {
    throw new Error(`Discovery ${label} fields do not match schema 1`);
  }
}

function bytesEqual(left: Uint8Array, right: Uint8Array) {
  return left.length === right.length && left.every((byte, index) => byte === right[index]);
}

function encodeBase64(bytes: Uint8Array) {
  let binary = '';
  for (let offset = 0; offset < bytes.length; offset += 0x8000) {
    binary += String.fromCharCode(...bytes.subarray(offset, offset + 0x8000));
  }
  return btoa(binary);
}

function decodeBase64(value: string) {
  const binary = atob(value);
  return Uint8Array.from(binary, (char) => char.charCodeAt(0));
}

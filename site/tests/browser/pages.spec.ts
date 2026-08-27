import { expect, test, type Page } from '@playwright/test'
import { createHash, createPrivateKey, createPublicKey, sign } from 'node:crypto'

const discoveryFixture = makeDiscoveryFixture()

test.beforeEach(async ({ page }) => {
  await page.route('**/discovery/**', async (route) => {
    const body = discoveryFixture.get(new URL(route.request().url()).pathname.split('/discovery/')[1] ?? '')
    await route.fulfill(body
      ? { body: Buffer.from(body), contentType: 'application/json', headers: { etag: '"browser-discovery-7"' } }
      : { status: 404, body: 'missing' })
  })
})

function observeFailures(page: Page) {
  const failures: string[] = []
  page.on('console', (message) => {
    if (message.type() === 'error') failures.push(`console: ${message.text()}`)
  })
  page.on('pageerror', error => failures.push(`page: ${error.message}`))
  page.on('requestfailed', request => failures.push(`request: ${request.method()} ${request.url()} (${request.failure()?.errorText ?? 'unknown'})`))
  page.on('response', (response) => {
    if (response.status() >= 400) failures.push(`response: ${response.status()} ${response.url()}`)
  })
  return failures
}

async function expectNoHorizontalOverflow(page: Page) {
  const widths = await page.evaluate(() => ({
    client: document.documentElement.clientWidth,
    scroll: document.documentElement.scrollWidth,
  }))
  expect(widths.scroll, `document is ${widths.scroll - widths.client}px wider than its viewport`).toBeLessThanOrEqual(widths.client + 1)
}

test('hydrates finalized CSP pages without runtime or layout failures', async ({ page }) => {
  const failures = observeFailures(page)
  for (const path of ['./', 'plugins', 'plugins/chrome-devtools']) {
    await page.goto(path)
    await expect(page.locator('main')).toBeVisible()
    await expect(page.locator('h1')).toHaveCount(1)
    await expect(page.locator('meta[http-equiv="Content-Security-Policy"]')).toHaveCount(1)
    await expectNoHorizontalOverflow(page)
  }
  expect(failures).toEqual([])
})

test('operates the target multi-select entirely by keyboard', async ({ page }) => {
  const failures = observeFailures(page)
  await page.goto('./')

  const trigger = page.getByRole('button', { name: /Choose target clients:/ })
  await expect(trigger).toHaveAttribute('data-hydrated', 'true')
  await trigger.focus()
  await trigger.press('Enter')
  const codex = page.getByRole('checkbox', { name: /Codex/ })
  await codex.focus()
  await codex.press('Space')
  await expect(codex).toBeChecked()
  await page.keyboard.press('Escape')
  await expect(trigger).toBeFocused()
  await expectNoHorizontalOverflow(page)
  expect(failures).toEqual([])
})

test('operates combobox and select catalog filters entirely by keyboard', async ({ page }) => {
  const failures = observeFailures(page)
  await page.goto('plugins')

  const category = page.getByRole('combobox', { name: 'Filter by category' })
  await expect(category).toHaveAttribute('data-hydrated', 'true')
  await category.focus()
  await category.press('End')
  await category.press('Shift+Home')
  await category.press('Backspace')
  await expect(category).toHaveValue('')
  await category.pressSequentially('docs')
  await category.press('ArrowDown')
  await category.press('Enter')
  await expect(category).toHaveValue('docs')

  const component = page.getByRole('combobox', { name: 'Filter by component' })
  await component.focus()
  await component.press('Space')
  await page.getByRole('option', { name: /^mcp/ }).press('Enter')
  await expect(component).toContainText('mcp')

  const source = page.getByRole('combobox', { name: 'Filter by source' })
  await source.focus()
  await source.press('Space')
  await page.getByRole('option', { name: /^Community bridges/ }).press('Enter')
  await expect(source).toContainText('Community bridges')

  const cards = page.locator('.plugin-card')
  await expect(cards).toHaveCount(1)
  await expectNoHorizontalOverflow(page)
  expect(failures).toEqual([])
})

test('keeps bridge alternatives on one product page', async ({ page }) => {
  const failures = observeFailures(page)
  await page.goto('plugins/chrome-devtools')
  await expect(page.locator('.distribution-list')).toContainText('Community bridge')
  await expect(page.getByRole('heading', { name: 'Product release history' })).toBeVisible()
  await expect(page.locator('.distribution-list > li')).toHaveCount(2)
  expect(failures).toEqual([])
})

test('renders target authentication distinctly and keeps it tied to multiselect targets', async ({ page }) => {
  const failures = observeFailures(page)
  await page.goto('plugins')

  const search = page.getByRole('searchbox', { name: 'Search plugins' })
  await search.fill('Agent Code Navigator')
  const navigator = page.locator('.plugin-card').filter({ hasText: 'Agent Code Navigator' })
  await expect(navigator.locator('.plugin-card__auth')).toHaveText('No account required')
  await navigator.getByRole('button', { name: /Choose clients for Agent Code Navigator:/ }).click()
  await page.getByRole('checkbox', { name: /Codex/ }).click()
  await page.keyboard.press('Escape')
  await expect(navigator.locator('.plugin-card__auth')).toHaveText('No account required')
  await expect(navigator.getByRole('button', { name: /Choose clients for Agent Code Navigator: 2 agents/ })).toBeVisible()

  await search.fill('Atlassian')
  const atlassian = page.locator('.plugin-card').filter({ hasText: 'Atlassian' })
  await expect(atlassian.locator('.plugin-card__auth')).toHaveText('Authentication required')

  await page.goto('plugins/atlassian')
  await expect(page.locator('.distribution-list')).toContainText('codex — Managed install; Authentication required')
  await page.goto('plugins/agent-code-navigator')
  await expect(page.locator('.distribution-list')).toContainText('codex — Managed install; No account required')
  await expectNoHorizontalOverflow(page)
  expect(failures).toEqual([])
})

test('unsigned pull-request preview exposes no copyable install command', async ({ page }) => {
  const failures = observeFailures(page)
  for (const path of ['./', 'plugins/chrome-devtools']) {
    await page.goto(path)
    await expect(page.getByRole('button', { name: /Copy command|Command copied/ })).toHaveCount(0)
    await expect(page.getByText(/Commands? unavailable.*review preview/i)).toBeVisible()
  }
  expect(failures).toEqual([])
})

test('loads, filters, and installs one signed unreviewed package without a site rebuild', async ({ page }) => {
  await page.goto('plugins')
  await expect(page.getByText(/2 unreviewed packages from signed index 7/)).toBeVisible()
  await page.getByRole('searchbox', { name: 'Search plugins' }).fill('portable-demo')
  const card = page.locator('.plugin-card').filter({ hasText: 'portable-demo' })
  await expect(card.getByText('Schema conformant · unreviewed').first()).toBeVisible()
  await expect(card).toContainText(`Immutable commit ${'a'.repeat(40)}`)
  await expect(card).toContainText(`Manifest sha256:${'2'.repeat(64)}`)
  await card.getByRole('button', { name: /Choose clients for portable-demo:/ }).click()
  await page.getByRole('checkbox', { name: /Codex/ }).click()
  await page.keyboard.press('Escape')
  await expect(card.locator('.command-snippet')).toContainText('npx universal-agent-plugins add discovery:example/portable//agent-plugin --target codex,cursor')

  await page.unroute('**/discovery/**')
  await page.route('**/discovery/**', route => route.abort('internetdisconnected'))
  await page.reload()
  await expect(page.getByText(/last-known-good signed index 7/)).toBeVisible()
  await page.getByRole('searchbox', { name: 'Search plugins' }).fill('portable-demo')
  await expect(page.locator('.plugin-card').filter({ hasText: 'portable-demo' })).toBeVisible()
})

test('fails closed for an unavailable discovered package and renders the generated 404 page', async ({ page }) => {
  await page.goto('plugins')
  await expect(page.getByText(/2 unreviewed packages from signed index 7/)).toBeVisible()
  await page.getByRole('searchbox', { name: 'Search plugins' }).fill('unavailable-demo')
  const card = page.locator('.plugin-card').filter({ hasText: 'unavailable-demo' })
  await expect(card.getByText('Unavailable · unreviewed')).toBeVisible()
  await expect(card.getByText(/no install command is generated/)).toBeVisible()
  await expect(card.getByRole('button', { name: /Copy command/ })).toHaveCount(0)

  const response = await page.goto('plugins/not-a-real-package')
  expect(response?.status()).toBe(404)
  await expect(page.getByRole('heading', { name: /Page not found/i })).toBeVisible()
})

function makeDiscoveryFixture() {
  const encoder = new TextEncoder()
  const canonical = (value: unknown) => {
    const sort = (item: unknown): unknown => Array.isArray(item)
      ? item.map(sort)
      : item && typeof item === 'object'
        ? Object.fromEntries(Object.entries(item).sort(([left], [right]) => left < right ? -1 : left > right ? 1 : 0).map(([key, child]) => [key, sort(child)]))
        : item
    return encoder.encode(`${JSON.stringify(sort(value))}\n`)
  }
  const digest = (bytes: Uint8Array) => `sha256:${createHash('sha256').update(bytes).digest('hex')}`
  const generated = new Date(Date.now() - 60_000)
  generated.setMilliseconds(0)
  const expires = new Date(generated.getTime() + 3 * 86_400_000)
  const generatedAt = generated.toISOString().replace('.000Z', 'Z')
  const expiresAt = expires.toISOString().replace('.000Z', 'Z')
  const compact = {
    slug: 'discovery:example/portable//agent-plugin', name: 'portable-demo', description: 'Portable test package', owner: 'example', repository: 'example/portable', package_path: 'agent-plugin', revision: 'a'.repeat(40), version: '1.2.3', license: 'Apache-2.0', schema_version: '1.0.0', components: { extensions: 0, mcp: 1, skills: 1 }, mcp_transports: ['stdio'], compatible_clients: ['codex', 'cursor'], authentication: 'unknown', status: 'conformant_unreviewed', runtime_reviewed: false, tree_digest: `sha256:${'1'.repeat(64)}`, manifest_digest: `sha256:${'2'.repeat(64)}`, stars: 412, repository_updated_at: generatedAt, reviewed_distribution_id: null, availability: 'available',
  }
  const unavailable = { ...compact, slug: 'discovery:example/unavailable//agent-plugin', name: 'unavailable-demo', repository: 'example/unavailable', revision: 'c'.repeat(40), availability: 'unavailable' }
  const compactRecords = [compact, unavailable]
  const search = { search_schema_version: 1, sequence: 7, generated_at: generatedAt, records: compactRecords }
  const searchBytes = canonical(search)
  const snapshot = { discovery_schema_version: 1, sequence: 7, publication_id: 'browser-test-7', source_commit: 'b'.repeat(40), generated_at: generatedAt, expires_at: expiresAt, complete: true, query_manifest_digest: `sha256:${'3'.repeat(64)}`, partitions: [{ query: 'path:plugin.json', size_min: 0, size_max: 1023, total_count: 2 }], search_projection: { path: 'search/00000000000000000007.json', digest: digest(searchBytes), record_count: 2 }, records: compactRecords.map(record => ({ ...record, author: { name: 'Example' }, first_seen: generatedAt, last_seen: generatedAt })) }
  const snapshotBytes = canonical(snapshot)
  const seed = Buffer.from(Array.from({ length: 32 }, (_, index) => index + 1))
  const privateKey = createPrivateKey({ key: Buffer.concat([Buffer.from('302e020100300506032b657004220420', 'hex'), seed]), format: 'der', type: 'pkcs8' })
  const rawPublic = createPublicKey(privateKey).export({ format: 'der', type: 'spki' }).subarray(-32).toString('base64')
  if (rawPublic !== 'ebVWLo/mVPlAeLES6KmLp5AfhTrmlb7X4OORC60ElmQ=') throw new Error('browser Discovery test key mismatch')
  const prefix = encoder.encode('UAP-DISCOVERY-INDEX-ED25519-V1\0')
  const signed = new Uint8Array(prefix.length + 8 + snapshotBytes.length)
  signed.set(prefix)
  new DataView(signed.buffer).setBigUint64(prefix.length, BigInt(snapshotBytes.length))
  signed.set(snapshotBytes, prefix.length + 8)
  const envelope = { envelope_schema_version: 1, snapshot_schema_version: 1, sequence: 7, key_id: 'test-discovery', algorithm: 'Ed25519', signature_domain: 'UAP-DISCOVERY-INDEX-ED25519-V1', snapshot_digest: digest(snapshotBytes), signature: sign(null, signed, privateKey).toString('base64') }
  const pointer = { pointer_schema_version: 1, snapshot_schema_version: 1, sequence: 7, snapshot_path: 'snapshots/00000000000000000007.json', envelope_path: 'snapshots/00000000000000000007.envelope.json', search_path: 'search/00000000000000000007.json', fetch_contract: { max_redirects: 0, latest_max_bytes: 16 << 10, snapshot_max_bytes: 16 << 20, envelope_max_bytes: 16 << 10, search_max_bytes: 10 << 20, retry_attempts: 1 } }
  return new Map([
    ['latest.json', canonical(pointer)],
    ['snapshots/00000000000000000007.json', snapshotBytes],
    ['snapshots/00000000000000000007.envelope.json', canonical(envelope)],
    ['search/00000000000000000007.json', searchBytes],
  ])
}

// Live production smoke only: no fixture routes, account sessions, or command execution.
import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { mkdir, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { chromium, expect } from '@playwright/test'

const base = new URL('https://777genius.github.io/universal-agent-plugins/')
const evidenceRoot = process.env.EVIDENCE_ROOT
assert(evidenceRoot, 'EVIDENCE_ROOT is required')
const digest = bytes => `sha256:${createHash('sha256').update(bytes).digest('hex')}`
const browser = await chromium.launch({ headless: true })
const evidence = { schema_version: 1, url: base.href, observed_at: new Date().toISOString(), viewports: [] }
try {
  for (const viewport of [{ width: 1440, height: 1000 }, { width: 390, height: 844 }]) {
    const context = await browser.newContext({ viewport, isMobile: viewport.width < 500, hasTouch: viewport.width < 500 })
    try {
      await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: base.origin })
      const page = await context.newPage()
      const errors = []
      page.on('pageerror', error => errors.push(error.message))
      page.on('console', message => { if (message.type() === 'error') errors.push(message.text()) })
      page.on('requestfailed', request => {
        const failure = request.failure()?.errorText
        // Navigating from Home to Plugins can cancel the previous page's redundant
        // background refresh after its signed last-known-good cache is already visible.
        const expectedNavigationAbort = failure === 'net::ERR_ABORTED' &&
          new URL(request.url()).pathname === '/universal-agent-plugins/discovery/latest.json'
        if (!expectedNavigationAbort) errors.push(`${request.url()}: ${failure}`)
      })
      page.on('response', response => { if (response.status() >= 400) errors.push(`${response.status()}: ${response.url()}`) })
      const fit = async () => assert(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1), 'horizontal overflow')
      for (const path of ['', 'plugins']) {
        assert.equal((await page.goto(new URL(path, base).href))?.status(), 200)
        await expect(page.locator('main')).toBeVisible()
        await expect(page.locator('h1')).toHaveCount(1)
        await expect(page.locator('meta[http-equiv="Content-Security-Policy"]')).toHaveCount(1)
        await expect(page.locator('[data-hydrated="true"]').first()).toBeVisible()
        // Wait before interacting: catalog intentionally defers Discovery replacement during focus.
        await expect(page.locator('.discovery-status--current, .discovery-status--cached')).toBeVisible({ timeout: 60_000 })
        await fit()
      }
      const registry = await page.evaluate(() => window.__NUXT__.config.public.registryIndex)
      assert.equal(registry.data_source, 'published_snapshot')
      assert(Number.isSafeInteger(registry.snapshot_sequence) && registry.snapshot_sequence >= 20)
      const get = async path => {
        const response = await context.request.get(new URL(path, base).href, { maxRedirects: 0, timeout: 30_000 })
        assert.equal(response.status(), 200)
        return response.body()
      }
      const stem = String(registry.snapshot_sequence).padStart(20, '0')
      const directoryRaw = await get(`registry/schemas/1/snapshots/${stem}.json`)
      const directoryEnvelope = JSON.parse(await get(`registry/schemas/1/snapshots/${stem}.envelope.json`))
      assert.equal(JSON.parse(directoryRaw).sequence, registry.snapshot_sequence)
      assert.equal(directoryEnvelope.sequence, registry.snapshot_sequence)
      assert.equal(directoryEnvelope.snapshot_digest, digest(directoryRaw))
      const cached = await page.evaluate(async () => {
        const entry = new URL('discovery/.browser-lkg.json', `${location.origin}/universal-agent-plugins/`)
        const response = await (await caches.open('uap-discovery-v1')).match(entry)
        if (!response?.ok) throw new Error('No signature-verified browser Discovery cache')
        return response.json()
      })
      const discoveryRaw = Buffer.from(cached.bytes.snapshot, 'base64')
      const discovery = JSON.parse(discoveryRaw)
      const discoveryEnvelope = JSON.parse(Buffer.from(cached.bytes.envelope, 'base64'))
      assert(Number.isSafeInteger(discovery.sequence) && discovery.sequence >= 20)
      assert(discovery.records.length >= 2000)
      assert.equal(discoveryEnvelope.sequence, discovery.sequence)
      assert.equal(discoveryEnvelope.snapshot_digest, digest(discoveryRaw))
      await expect(page.locator('.discovery-status')).toContainText(new RegExp(`${discovery.records.length} unreviewed packages from (?:last-known-good )?signed index ${discovery.sequence}(?:\\D|$)`))
      const search = page.getByRole('searchbox', { name: 'Search plugins' })
      const checkCard = async (query, card, selector) => {
        await search.fill(query)
        await expect(card).toHaveCount(1)
        await expect(card).toBeVisible()
        await card.getByRole('button', { name: /Choose clients for/ }).click()
        for (const name of ['Codex', 'Cursor', 'Kiro']) {
          const checkbox = page.getByRole('checkbox', { name: new RegExp(`^${name}(?:\\s|$)`) })
          if (await checkbox.getAttribute('aria-checked') !== 'true') await checkbox.click()
          await expect(checkbox).toBeChecked()
        }
        await page.keyboard.press('Escape')
        const command = `npx universal-agent-plugins add ${selector} --target codex,cursor,kiro`
        await expect(card.locator('.command-snippet code')).toHaveText(command)
        await card.getByRole('button', { name: 'Copy command', exact: true }).click()
        await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(command)
        await fit()
        return command
      }
      const chrome = page.locator('.plugin-card')
        .filter({ has: page.locator('a[href$="/plugins/chrome-devtools"]') })
        .filter({ has: page.locator('.source-pill', { hasText: /^Install candidate/ }) })
      const chromeCommand = await checkCard('Chrome DevTools', chrome, 'chrome-devtools')
      await expect(chrome).toContainText('Install candidate v1.7.0-uap.1 · release 2')
      await expect(chrome).toContainText('signed fallback for selected clients')
      const selector = 'discovery:upstash/context7//plugins/agent-plugins/context7'
      const context7 = page.locator('.plugin-card').filter({ has: page.locator('.command-snippet code', { hasText: selector }) })
      const context7Command = await checkCard('context7', context7, selector)
      await expect(context7).toContainText('Schema conformant · unreviewed')
      assert.deepEqual(errors, [])
      evidence.viewports.push({ viewport, directory_sequence: registry.snapshot_sequence,
        directory_snapshot_digest: digest(directoryRaw), discovery_sequence: discovery.sequence,
        discovery_snapshot_digest: digest(discoveryRaw), discovery_records: discovery.records.length,
        chrome_command: chromeCommand, context7_command: context7Command, ui_errors: errors,
        commands_executed: false, clipboard_verified: true })
    } finally { await context.close() }
  }
} finally { await browser.close() }
await mkdir(evidenceRoot, { recursive: true })
const body = `${JSON.stringify(evidence, null, 2)}\n`
await writeFile(join(evidenceRoot, 'public-site.json'), body)
await writeFile(join(evidenceRoot, 'public-site.sha256'), `${digest(body).slice(7)}  public-site.json\n`)

import { expect, test } from '@playwright/test';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { clients } from '../../data/clients';

// Software WebGL is a test-browser capability, not a product configuration.
test.use({ launchOptions: { args: ['--enable-unsafe-swiftshader'] } });

let sceneFile: string;
test.beforeAll(async () => {
  const { default: manifest } = await import(
    pathToFileURL(resolve('.nuxt/dist/server/client.manifest.mjs')).href
  );
  const entry = Object.values(manifest as Record<string, { name?: string; file: string }>).find(
    (value) => value.name === 'heroScene',
  );
  expect(entry, 'production build must keep the optional scene in its own lazy chunk').toBeTruthy();
  sceneFile = entry!.file;
});
const isSceneRequest = (url: string) => new URL(url).pathname.endsWith(`/${sceneFile}`);

test('static orbit includes every official logo, to the right and above installation', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.emulateMedia({ reducedMotion: 'reduce' });
  const sceneRequests: string[] = [];
  page.on('request', (request) => {
    if (isSceneRequest(request.url())) sceneRequests.push(request.url());
  });
  await page.goto('./');
  const field = page.locator('.hero__demo .hero-agent-field');
  await expect(field).toHaveAttribute('aria-hidden', 'true');
  await expect(field).toHaveAttribute('data-state', 'fallback');
  await expect(field.locator('[data-client-id]')).toHaveCount(clients.length);
  expect(
    await field
      .locator('[data-client-id]')
      .evaluateAll((nodes) => nodes.map((node) => node.getAttribute('data-client-id'))),
  ).toEqual(clients.map((client) => client.id));
  for (const client of clients) {
    const logo = field.locator(`[data-client-id="${client.id}"] img`);
    await expect(logo).toHaveAttribute('src', new RegExp(`/client-icons/${client.icon}$`));
    await expect
      .poll(() =>
        logo.evaluate((image: HTMLImageElement) => image.complete && image.naturalWidth > 0),
      )
      .toBe(true);
  }
  const copy = (await page.locator('.hero__copy').boundingBox())!;
  const scene = (await field.boundingBox())!;
  const installation = (await page.locator('.hero__window').boundingBox())!;
  expect(scene.x).toBeGreaterThan(copy.x + copy.width);
  expect(scene.y + scene.height).toBeLessThanOrEqual(installation.y);
  expect(await field.evaluate((node) => getComputedStyle(node).pointerEvents)).toBe('none');
  expect(sceneRequests).toEqual([]);
});

test('mobile renders the light fallback without downloading the 3D scene', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const sceneRequests: string[] = [];
  page.on('request', (request) => {
    if (isSceneRequest(request.url())) sceneRequests.push(request.url());
  });
  await page.goto('./');
  const field = page.locator('.hero-agent-field');
  await expect(field).toHaveAttribute('data-state', 'fallback');
  await field.scrollIntoViewIfNeeded();
  await expect(page.locator('.hero__demo .app-multiselect__trigger')).toHaveAttribute(
    'data-hydrated',
    'true',
  );
  await expect(field.locator('[data-client-id]')).toHaveCount(clients.length);
  await expect(field.locator('canvas')).toHaveCount(0);
  expect(sceneRequests).toEqual([]);
});

test('fallback fits every supported screen without overlapping the command card', async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('./');
  for (const width of [1440, 1280, 1024, 800, 390, 280]) {
    await page.setViewportSize({ width, height: 1000 });
    const field = page.locator('.hero-agent-field');
    await field.scrollIntoViewIfNeeded();
    const scene = (await field.boundingBox())!;
    const installation = (await page.locator('.hero__window').boundingBox())!;
    expect(scene.x, `${width}px`).toBeGreaterThanOrEqual(0);
    expect(scene.x + scene.width, `${width}px`).toBeLessThanOrEqual(width);
    expect(scene.y + scene.height, `${width}px`).toBeLessThanOrEqual(installation.y);
    for (const node of await field.locator('[data-client-id]').all()) {
      const rect = (await node.boundingBox())!;
      expect(rect.x, `${width}px`).toBeGreaterThanOrEqual(scene.x - 1);
      expect(rect.x + rect.width, `${width}px`).toBeLessThanOrEqual(scene.x + scene.width + 1);
      expect(rect.y, `${width}px`).toBeGreaterThanOrEqual(scene.y - 1);
      expect(rect.y + rect.height, `${width}px`).toBeLessThanOrEqual(scene.y + scene.height + 1);
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
  }
});

test('unavailable WebGL preserves the fallback and the working install command', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.addInitScript(() => {
    const original = HTMLCanvasElement.prototype.getContext;
    HTMLCanvasElement.prototype.getContext = function (type: string, ...args: unknown[]) {
      if (type === 'webgl' || type === 'webgl2' || type === 'experimental-webgl') return null;
      return original.apply(this, [type, ...args] as Parameters<typeof original>);
    } as typeof original;
  });
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await page.goto('./');
  await expect(page.locator('.hero__plugin-count')).toContainText(/[\d,]+/, { timeout: 15_000 });
  await expect(page.locator('.hero-agent-field')).toHaveAttribute('data-state', 'fallback');
  await expect(page.locator('.hero-agent-field [data-client-id]')).toHaveCount(clients.length);
  await page
    .getByRole('button', { name: 'Choose target agents: All installed agents', exact: true })
    .click();
  await page.getByRole('checkbox', { name: /^Cursor / }).click();
  await page.keyboard.press('Escape');
  await expect(page.locator('.hero__demo .command-snippet')).toContainText('--target cursor');
  expect(errors).toEqual([]);
});

test.describe('3D scene lifecycle', () => {
  test('sustained synchronous render overload permanently restores the light fallback', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.addInitScript((sceneChunk) => {
      const metrics = { requests: 0 };
      Object.assign(window, { sceneTestMetrics: metrics });
      const request = window.requestAnimationFrame;
      window.requestAnimationFrame = function (callback) {
        if (new Error().stack?.includes(sceneChunk)) metrics.requests++;
        return request.call(this, callback);
      };
      const clear = WebGL2RenderingContext.prototype.clear;
      let delayedFrames = 0;
      WebGL2RenderingContext.prototype.clear = function (mask) {
        if (
          this.canvas instanceof HTMLCanvasElement &&
          this.canvas.className === 'hero-agent-field__canvas' &&
          delayedFrames < 60
        ) {
          delayedFrames++;
          const deadline = performance.now() + 25;
          while (performance.now() < deadline) {
            /* Test-only bounded CPU pressure. */
          }
        }
        return clear.call(this, mask);
      };
    }, sceneFile);
    const errors: string[] = [];
    page.on('pageerror', (error) => errors.push(error.message));
    await page.goto('./');
    const field = page.locator('.hero-agent-field');
    await expect(field).toHaveAttribute('data-state', 'ready', { timeout: 20_000 });
    await expect(field).toHaveAttribute('data-state', 'fallback', { timeout: 10_000 });
    await expect(field).toHaveAttribute('data-running', 'false');
    await expect(field.locator('canvas')).toHaveCount(0);
    await expect(field.locator('[data-client-id]')).toHaveCount(clients.length);
    const readRequests = () =>
      page.evaluate(
        () =>
          (window as Window & { sceneTestMetrics: { requests: number } }).sceneTestMetrics.requests,
      );
    const stopped = await readRequests();
    expect(stopped).toBeGreaterThan(0);
    await page.waitForTimeout(300);
    expect(await readRequests()).toBe(stopped);
    // Visibility changes must not restart an enhancement rejected for CPU cost.
    await page.getByRole('contentinfo').scrollIntoViewIfNeeded();
    await field.scrollIntoViewIfNeeded();
    await page.waitForTimeout(300);
    await expect(field).toHaveAttribute('data-state', 'fallback');
    await expect(field.locator('canvas')).toHaveCount(0);
    await page
      .getByRole('button', { name: 'Choose target agents: All installed agents', exact: true })
      .click();
    await page.getByRole('checkbox', { name: /^Cursor / }).click();
    await page.keyboard.press('Escape');
    await expect(page.locator('.hero__demo .command-snippet')).toContainText('--target cursor');
    expect(errors).toEqual([]);
  });

  test('deferred scene renders at bounded resolution and pauses offscreen', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.addInitScript(() => {
      const clear = WebGL2RenderingContext.prototype.clear;
      WebGL2RenderingContext.prototype.clear = function (mask) {
        if (
          this.canvas instanceof HTMLCanvasElement &&
          this.canvas.className === 'hero-agent-field__canvas'
        ) {
          const canvas = this.canvas as HTMLCanvasElement & { testFrames?: number };
          canvas.testFrames = (canvas.testFrames || 0) + 1;
        }
        return clear.call(this, mask);
      };
    });
    const errors: string[] = [];
    page.on('pageerror', (error) => errors.push(error.message));
    await page.goto('./');
    const field = page.locator('.hero-agent-field');
    await expect(page.locator('.hero__demo .command-snippet')).toContainText('agentplugins add');
    await expect(field).toHaveAttribute('data-state', 'ready', { timeout: 20_000 });
    await expect(field).toHaveAttribute('data-running', 'true');
    const size = await field.locator('canvas').evaluate((canvas: HTMLCanvasElement) => ({
      width: canvas.width,
      height: canvas.height,
      cssWidth: canvas.clientWidth,
      cssHeight: canvas.clientHeight,
    }));
    expect(size.width).toBeGreaterThan(0);
    expect(size.width).toBeLessThanOrEqual(size.cssWidth * 1.5 + 1);
    expect(size.height).toBeLessThanOrEqual(size.cssHeight * 1.5 + 1);
    const readFrames = () =>
      field
        .locator('canvas')
        .evaluate((canvas: HTMLCanvasElement & { testFrames?: number }) => canvas.testFrames || 0);
    const started = await readFrames();
    await page.waitForTimeout(1000);
    const rendered = (await readFrames()) - started;
    expect(rendered).toBeGreaterThan(0);
    expect(rendered).toBeLessThanOrEqual(32);
    await page.getByRole('contentinfo').scrollIntoViewIfNeeded();
    await expect(field).toHaveAttribute('data-running', 'false');
    const pausedFrames = await readFrames();
    await page.waitForTimeout(250);
    expect(await readFrames()).toBe(pausedFrames);
    await field.scrollIntoViewIfNeeded();
    await expect(field).toHaveAttribute('data-running', 'true');
    await page.evaluate(() => {
      Object.defineProperty(document, 'hidden', { value: true, configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    await expect(field).toHaveAttribute('data-running', 'false');
    await page.evaluate(() => {
      Reflect.deleteProperty(document, 'hidden');
      document.dispatchEvent(new Event('visibilitychange'));
    });
    await expect(field).toHaveAttribute('data-running', 'true');
    await page.emulateMedia({ reducedMotion: 'reduce' });
    await expect(field).toHaveAttribute('data-running', 'false');
    await expect(field).toHaveAttribute('data-state', 'fallback');
    expect(errors).toEqual([]);
  });

  test('context loss falls back cleanly and navigation leaves no scene behind', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 1000 });
    const errors: string[] = [];
    page.on('pageerror', (error) => errors.push(error.message));
    await page.goto('./');
    const field = page.locator('.hero-agent-field');
    await expect(field).toHaveAttribute('data-state', 'ready', { timeout: 20_000 });
    await field
      .locator('canvas')
      .evaluate((canvas) =>
        canvas.dispatchEvent(new Event('webglcontextlost', { cancelable: true })),
      );
    await expect(field).toHaveAttribute('data-state', 'fallback');
    await expect(field).toHaveAttribute('data-running', 'false');
    await expect(field.locator('canvas')).toHaveCount(0);
    await page.locator('.client-strip a[href$="/agents/codex"]').click();
    await expect(
      page.getByRole('heading', { name: 'Install Agent Plugins for Codex', exact: true }),
    ).toBeVisible();
    await expect(field).toHaveCount(0);
    expect(errors).toEqual([]);
  });

  test('a failed optional scene download does not break the page', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 1000 });
    let blocked = false;
    await page.route(
      (url) => isSceneRequest(url.href),
      (route) => {
        blocked = true;
        return route.abort();
      },
    );
    const errors: string[] = [];
    page.on('pageerror', (error) => errors.push(error.message));
    await page.goto('./');
    await expect(page.locator('.hero__plugin-count')).toContainText(/[\d,]+/, { timeout: 15_000 });
    await expect.poll(() => blocked, { timeout: 15_000 }).toBe(true);
    await expect(page.locator('.hero-agent-field')).toHaveAttribute('data-state', 'fallback');
    await expect(page.locator('.hero__demo .command-snippet')).toContainText('agentplugins add');
    expect(errors).toEqual([]);
  });
});

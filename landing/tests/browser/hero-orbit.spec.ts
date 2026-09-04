import { expect, test } from '@playwright/test';
import { clients } from '../../data/clients';

test('CSS orbit is visible without JavaScript and keeps every logo on the circle', async ({ browser, baseURL }) => {
  const context = await browser.newContext({ baseURL, javaScriptEnabled: false, reducedMotion: 'reduce' });
  const page = await context.newPage();
  try {
    await page.goto('./');
    const field = page.locator('.hero__demo .hero-agent-field');
    await expect(field).toHaveAttribute('aria-hidden', 'true');
    await expect(field.locator('[data-client-id]')).toHaveCount(clients.length);
    for (const client of clients) {
      const logo = field.locator(`[data-client-id="${client.id}"] img`);
      await expect(logo).toHaveAttribute('src', new RegExp(`/client-icons/${client.icon}$`));
      await expect.poll(() => logo.evaluate((image: HTMLImageElement) => image.complete && image.naturalWidth > 0)).toBe(true);
    }
    // Flatten the camera only; retain the historical spoke/circumference geometry.
    await field.locator('.hero-agent-field__plane').evaluate((node: HTMLElement) => {
      node.style.transform = 'none';
    });
    const error = await field.evaluate((element) => {
      const track = element.querySelector('.hero-agent-field__track')!.getBoundingClientRect();
      return Math.max(...[...element.querySelectorAll('.hero-agent-field__node')].map((node) => {
        const rect = node.getBoundingClientRect();
        return Math.abs(Math.hypot(rect.x + rect.width / 2 - track.x - track.width / 2,
          rect.y + rect.height / 2 - track.y - track.height / 2) - track.width / 2);
      }));
    });
    expect(error).toBeLessThan(2);
    await expect(field.locator('canvas')).toHaveCount(0);
  } finally {
    await context.close();
  }
});

test('historical CSS depth animates on the right and pauses offscreen or hidden', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto('./');
  const field = page.locator('.hero__demo .hero-agent-field');
  const track = field.locator('.hero-agent-field__track');
  await expect(field).toHaveClass(/hero-agent-field--active/);
  const first = await track.evaluate((node) => getComputedStyle(node).transform);
  await expect.poll(() => track.evaluate((node) => getComputedStyle(node).transform)).not.toBe(first);
  expect(await field.locator('.hero-agent-field__plane').evaluate((node) => getComputedStyle(node).transform)).toMatch(/^matrix3d\(/);
  expect(await field.evaluate((node) => getComputedStyle(node).pointerEvents)).toBe('none');
  const copy = (await page.locator('.hero__copy').boundingBox())!;
  const bounds = (await field.boundingBox())!;
  const installation = (await page.locator('.hero__window').boundingBox())!;
  expect(bounds.x).toBeGreaterThan(copy.x + copy.width);
  expect(bounds.y + bounds.height).toBeLessThanOrEqual(installation.y);
  await page.getByRole('contentinfo').scrollIntoViewIfNeeded();
  await expect(field).not.toHaveClass(/hero-agent-field--active/);
  expect(await track.evaluate((node) => getComputedStyle(node).animationPlayState)).toBe('paused');
  await field.scrollIntoViewIfNeeded();
  await expect(field).toHaveClass(/hero-agent-field--active/);
  await page.evaluate(() => {
    Object.defineProperty(document, 'hidden', { value: true, configurable: true });
    document.dispatchEvent(new Event('visibilitychange'));
  });
  await expect(field).not.toHaveClass(/hero-agent-field--active/);
  await page.evaluate(() => {
    Reflect.deleteProperty(document, 'hidden');
    document.dispatchEvent(new Event('visibilitychange'));
  });
  await expect(field).toHaveClass(/hero-agent-field--active/);
  await page.emulateMedia({ reducedMotion: 'reduce' });
  expect(await field.evaluate((node) => node.getAnimations({ subtree: true }).length)).toBe(0);
});

test('all logos stay within the scene and clear the command throughout responsive rotation', async ({ page }) => {
  await page.goto('./');
  const field = page.locator('.hero-agent-field');
  await expect(field).toHaveClass(/hero-agent-field--active/);
  for (const width of [1440, 1280, 1024, 800, 390, 280]) {
    await page.setViewportSize({ width, height: 1000 });
    await field.scrollIntoViewIfNeeded();
    for (const time of [0, 9000, 18000, 36000, 54000]) {
      await field.evaluate((node, elapsed) => {
        for (const animation of node.getAnimations({ subtree: true })) {
          animation.pause();
          animation.currentTime = elapsed;
        }
      }, time);
      await page.evaluate(() => new Promise(requestAnimationFrame));
      const scene = (await field.boundingBox())!;
      const installation = (await page.locator('.hero__window').boundingBox())!;
      for (const node of await field.locator('.hero-agent-field__node').all()) {
        const rect = (await node.boundingBox())!;
        const label = `${width}px, ${time}ms`;
        expect(rect.x, label).toBeGreaterThanOrEqual(Math.max(0, scene.x));
        expect(rect.x + rect.width, label).toBeLessThanOrEqual(Math.min(width, scene.x + scene.width));
        expect(rect.y, label).toBeGreaterThanOrEqual(scene.y);
        expect(rect.y + rect.height, label).toBeLessThanOrEqual(scene.y + scene.height);
        expect(rect.y + rect.height, label).toBeLessThan(installation.y);
      }
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    }
  }
});

test('installation works with WebGL forbidden and the orbit has no canvas or optional renderer', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.addInitScript(() => {
    const original = HTMLCanvasElement.prototype.getContext;
    HTMLCanvasElement.prototype.getContext = function (type: string, ...args: unknown[]) {
      if (/webgl/i.test(type)) throw new Error('This CSS-only page must not create WebGL');
      return original.apply(this, [type, ...args] as Parameters<typeof original>);
    } as typeof original;
  });
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await page.goto('./');
  const field = page.locator('.hero-agent-field');
  await expect(field).toHaveClass(/hero-agent-field--active/);
  await expect(field.locator('canvas')).toHaveCount(0);
  await page.getByRole('button', { name: 'Choose target agents: All installed agents', exact: true }).click();
  await page.getByRole('checkbox', { name: /^Cursor / }).click();
  await page.keyboard.press('Escape');
  await expect(page.locator('.hero__demo .command-snippet')).toContainText('--target cursor');
  await page.getByRole('button', { name: 'Toggle theme', exact: true }).click();
  await expect(field.locator('[data-client-id]')).toHaveCount(clients.length);
  expect(errors).toEqual([]);
});

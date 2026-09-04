import { expect, test } from '@playwright/test';
import { clients } from '../../data/clients';

test('hero orbit reuses every supported client and places logos on one circumference', async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('./');
  const field = page.locator('.hero__copy .hero-agent-field');
  await expect(field).toHaveAttribute('aria-hidden', 'true');
  await expect(field.locator('.hero-agent-field__node')).toHaveCount(clients.length);
  expect(
    await field
      .locator('[data-client-id]')
      .evaluateAll((nodes) => nodes.map((node) => node.getAttribute('data-client-id'))),
  ).toEqual(clients.map((client) => client.id));
  for (const client of clients) {
    const logo = field.locator(`[data-client-id="${client.id}"] img`);
    await expect(logo).toHaveAttribute('src', new RegExp(`/client-icons/${client.icon}$`));
    expect(
      await logo.evaluate((image: HTMLImageElement) => image.complete && image.naturalWidth > 0),
    ).toBe(true);
  }

  // Flatten only the camera for this geometry check, not the radial placement.
  await field.locator('.hero-agent-field__plane').evaluate((node: HTMLElement) => {
    node.style.transform = 'none';
  });
  await expect
    .poll(() =>
      field
        .locator('.hero-agent-field__plane')
        .evaluate((node) => getComputedStyle(node).transform),
    )
    .toBe('none');
  const radii = await field.evaluate((element) => {
    const track = element.querySelector('.hero-agent-field__track')!.getBoundingClientRect();
    const center = { x: track.x + track.width / 2, y: track.y + track.height / 2 };
    return [...element.querySelectorAll('.hero-agent-field__node')].map((node) => {
      const rect = node.getBoundingClientRect();
      return Math.abs(
        Math.hypot(rect.x + rect.width / 2 - center.x, rect.y + rect.height / 2 - center.y) -
          track.width / 2,
      );
    });
  });
  expect(Math.max(...radii)).toBeLessThan(2);
});

test('hero orbit animates with depth, stays left of installation, and pauses offscreen', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto('./');
  const field = page.locator('.hero-agent-field');
  await expect(field).toHaveClass(/hero-agent-field--active/);
  const track = field.locator('.hero-agent-field__track');
  const firstTransform = await track.evaluate((node) => getComputedStyle(node).transform);
  await expect
    .poll(() => track.evaluate((node) => getComputedStyle(node).transform))
    .not.toBe(firstTransform);
  expect(
    await field
      .locator('.hero-agent-field__plane')
      .evaluate((node) => getComputedStyle(node).transform),
  ).toMatch(/^matrix3d\(/);
  expect(await field.evaluate((node) => getComputedStyle(node).pointerEvents)).toBe('none');

  const installation = (await page.locator('.hero__demo').boundingBox())!;
  for (const node of await field.locator('.hero-agent-field__node').all()) {
    const rect = (await node.boundingBox())!;
    expect(rect.x).toBeGreaterThanOrEqual(0);
    expect(rect.x + rect.width).toBeLessThan(installation.x);
  }
  await page.getByRole('contentinfo').scrollIntoViewIfNeeded();
  await expect(field).not.toHaveClass(/hero-agent-field--active/);
  expect(await track.evaluate((node) => getComputedStyle(node).animationPlayState)).toBe('paused');
  await page.getByRole('heading', { name: 'One plugin All your agents' }).scrollIntoViewIfNeeded();
  await expect(field).toHaveClass(/hero-agent-field--active/);
});

test('mobile orbit keeps every logo visible and honors reduced motion', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('./');
  const field = page.locator('.hero-agent-field');
  await expect(field.locator('.hero-agent-field__node')).toHaveCount(clients.length);
  expect(await field.evaluate((node) => node.getAnimations({ subtree: true }).length)).toBe(0);
  const installation = (await page.locator('.hero__demo').boundingBox())!;
  for (const node of await field.locator('.hero-agent-field__node').all()) {
    const rect = (await node.boundingBox())!;
    expect(rect.x).toBeGreaterThanOrEqual(0);
    expect(rect.x + rect.width).toBeLessThanOrEqual(390);
    expect(rect.y + rect.height).toBeLessThan(installation.y);
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
});

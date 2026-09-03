import { expect, test } from '@playwright/test';

test('homepage installs with auto-detection and exposes the full directory', async ({ page }) => {
  const errors: string[] = [];
  page.on('console', (message) => {
    if (message.type() === 'error') errors.push(message.text());
  });
  page.on('response', (response) => {
    if (response.status() >= 400) errors.push(`${response.status()} ${response.url()}`);
  });

  await page.goto('./');
  await expect(page.getByRole('heading', { name: /One plugin/ })).toBeVisible();
  await expect(page.locator('.command-snippet').first()).not.toContainText('--target');
  await expect(page.getByText('Supported clients')).toBeVisible();
  await expect(page.locator('.client-strip li')).toHaveCount(11);
  await expect(page.locator('.plugin-card').first()).toBeVisible();
  await expect(page.getByText(/community packages found on GitHub/i)).toBeVisible({
    timeout: 15_000,
  });
  expect(errors).toEqual([]);
});

test('directory filters and reviewed detail keep automatic detection as the default', async ({
  page,
}) => {
  await page.goto('./plugins');
  await page.getByPlaceholder(/Search by name/).fill('gitlab');
  const gitlabCard = page
    .locator('.plugin-card')
    .filter({ has: page.getByRole('heading', { name: 'GitLab', exact: true }) });
  await expect(gitlabCard).toHaveCount(1);
  await Promise.all([
    page.waitForURL(/\/plugins\/gitlab$/),
    gitlabCard.getByRole('link', { name: 'GitLab', exact: true }).click(),
  ]);
  await expect(page.getByRole('heading', { name: 'GitLab', exact: true })).toBeVisible();
  await expect(page.getByText('All installed agents')).toBeVisible();
  await expect(page.locator('.command-snippet').first()).not.toContainText('--target');
});

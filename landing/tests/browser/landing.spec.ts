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
  await expect(page.getByRole('heading', { name: 'One plugin All your agents' })).toBeVisible();
  await expect(page.locator('.command-snippet').first()).not.toContainText('--target');
  await expect(page.getByText('Supported clients')).toBeVisible();
  await expect(page.locator('.client-strip li')).toHaveCount(11);
  await expect(page.locator('.plugin-card').first()).toBeVisible();
  await expect(page.locator('.catalog-count')).toContainText(/[2-9]\d{3} total/, {
    timeout: 15_000,
  });
  await expect(page.getByRole('link', { name: /Explore [\d,]+ plugins/ })).toBeVisible();
  await expect(page.getByText(/recently found community packages/i)).toHaveCount(0);
  expect(errors).toEqual([]);
});

test('community plugin titles open an installable plugin page instead of GitHub', async ({
  page,
}) => {
  await page.goto('./');
  const communityCard = page.locator('.plugin-card[data-trust="community"]').first();
  await expect(communityCard).toBeVisible({ timeout: 15_000 });
  const title = communityCard.locator('.plugin-card__title-link');
  const pluginName = (await title.textContent())?.trim();
  await expect(title).toHaveAttribute('href', /\/plugins\/community\?source=/);
  await title.click();
  await expect(page).toHaveURL(/\/plugins\/community\?source=/);
  await expect(page.getByRole('heading', { name: pluginName, exact: true })).toBeVisible({
    timeout: 15_000,
  });
  await expect(page.locator('.install-panel')).toBeVisible();
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

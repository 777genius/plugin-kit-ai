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
  await expect(page.locator('link[rel="icon"][type="image/svg+xml"]')).toHaveAttribute(
    'href',
    /icon\.svg$/,
  );
  await expect(page.locator('.app-logo__mark')).toHaveAttribute('src', /icon\.svg$/);
  await expect(page.locator('.hero-agent-field__hub img')).toHaveAttribute('src', /icon\.svg$/);
  await expect(page.getByRole('heading', { name: /One plugin/ })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'One plugin All your agents' })).toBeVisible();
  await expect(page.locator('.command-snippet').first()).toContainText('agentplugins add');
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
    page.waitForURL(/\/plugins\/gitlab\/?$/),
    gitlabCard.getByRole('link', { name: 'GitLab', exact: true }).click(),
  ]);
  await expect(page.getByRole('heading', { name: 'GitLab', exact: true })).toBeVisible();
  await expect(page.getByText('All installed agents')).toBeVisible();
  await expect(page.getByRole('link', { name: 'Native Go CLI · no Node.js' })).toBeVisible();
  await expect(page.locator('.command-snippet').first()).toContainText('agentplugins add');
  await expect(page.locator('.command-snippet').first()).not.toContainText('--target');
});

test('download page recommends the detected OS path and preserves the npx alternative', async ({
  browser,
  baseURL,
}) => {
  const context = await browser.newContext({
    userAgent:
      'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 ' +
      '(KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36',
  });
  const page = await context.newPage();

  await page.goto(new URL('download', baseURL).href);
  const tabs = page.locator('.download-section__install-tab');
  const script = tabs.filter({ hasText: 'Verified install script' });
  await expect(script).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByText('Best option for Linux selected')).toBeVisible();
  await expect(page.getByText(/install\.sh \| sh/)).toBeVisible();
  await expect(
    page.getByText('$HOME/.local/bin/agentplugins add context7 --target codex,cursor'),
  ).toBeVisible();

  await tabs.filter({ hasText: 'npx' }).click();
  await expect(
    page.getByText('npx universal-agent-plugins add context7 --target codex,cursor'),
  ).toBeVisible();
  await expect(page.getByText('npx universal-agent-plugins version')).toHaveCount(0);

  await context.close();
});

test('Windows visitors receive the PowerShell installer and invocation', async ({
  browser,
  baseURL,
}) => {
  const context = await browser.newContext({
    userAgent:
      'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 ' +
      '(KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36',
  });
  const page = await context.newPage();

  await page.goto(new URL('download', baseURL).href);
  const powershell = page
    .locator('.download-section__install-tab')
    .filter({ hasText: 'PowerShell' });
  await expect(powershell).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByText('Best option for Windows selected')).toBeVisible();
  await expect(page.getByText(/install\.ps1 \| iex/)).toBeVisible();
  await expect(
    page.getByText('& "$HOME\\.local\\bin\\agentplugins.exe" add context7 --target codex,cursor'),
  ).toBeVisible();

  await context.close();
});

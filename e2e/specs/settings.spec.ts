import { expect, test } from '../harness/test';
import { openHome, openResource } from '../harness/app';
import type { Page } from '@playwright/test';

async function openSettings(page: Page): Promise<void> {
  await openHome(page);
  await page.getByRole('button', { name: 'Settings' }).click();
  await expect(page.getByRole('dialog', { name: 'Settings' })).toBeVisible();
}

async function openSection(page: Page, section: string): Promise<void> {
  await openSettings(page);
  await page
    .getByRole('navigation', { name: 'Settings sections' })
    .getByRole('button', { name: section, exact: true })
    .click();
}

function settingsWrite(page: Page) {
  return page.waitForResponse(
    (response) =>
      response.url().includes('/api/settings') && response.request().method() === 'PUT',
    { timeout: 30_000 },
  );
}

function surface(page: Page): Promise<string> {
  return page.evaluate(() => {
    const root = document.documentElement;
    const style = getComputedStyle(root);
    return `${root.dataset.theme ?? ''}|${style.getPropertyValue('--surface').trim()}|${style.getPropertyValue('--fg').trim()}`;
  });
}

test.afterAll(async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();
  await openHome(page);
  const saved = page.waitForResponse(
    (response) =>
      response.url().includes('/api/settings') && response.request().method() === 'PUT',
    { timeout: 30_000 },
  );
  await page.getByRole('button', { name: 'Settings' }).click();
  await page.getByRole('combobox', { name: 'Theme preference' }).selectOption({ label: 'Dark' });
  await saved.catch(() => undefined);
  await context.close();
});

test('the dialog opens on the cog and closes on escape', async ({ page }) => {
  await openSettings(page);
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog', { name: 'Settings' })).toBeHidden();
});

test('the dialog names every section it can show', async ({ page }) => {
  await openSettings(page);
  const sections = page.getByRole('navigation', { name: 'Settings sections' });
  for (const name of ['Appearance', 'Cluster', 'Logs', 'Terminal', 'Panels', 'Keyboard', 'About']) {
    await expect(sections.getByRole('button', { name, exact: true })).toBeVisible();
  }
});

test('picking a theme repaints the page, not just the dropdown', async ({ page }) => {
  await openSettings(page);
  const before = await surface(page);
  await page.getByRole('combobox', { name: 'Theme preference' }).selectOption({ label: 'Light' });
  await expect.poll(() => surface(page), { timeout: 20_000 }).not.toBe(before);
  const light = await surface(page);
  await page.getByRole('combobox', { name: 'Theme preference' }).selectOption({ label: 'Matrix' });
  await expect.poll(() => surface(page), { timeout: 20_000 }).not.toBe(light);
});

test('the theme survives a reload because spinoza kept it', async ({ page }) => {
  await openSettings(page);
  const saved = page.waitForResponse(
    (response) =>
      response.url().includes('/api/settings') && response.request().method() === 'PUT',
    { timeout: 30_000 },
  );
  await page.getByRole('combobox', { name: 'Theme preference' }).selectOption({ label: 'Nord' });
  const nord = await surface(page);
  await saved;
  await page.reload();
  await page.waitForLoadState('domcontentloaded');
  await expect.poll(() => surface(page), { timeout: 20_000 }).toBe(nord);
  await page.getByRole('button', { name: 'Settings' }).click();
  await expect(
    page.getByRole('combobox', { name: 'Theme preference' }).locator('option:checked'),
  ).toHaveText('Nord');
});

test('a theme pasted as json is taken on and applied', async ({ page }) => {
  await openSettings(page);
  await page.getByRole('combobox', { name: 'Theme preference' }).selectOption({ label: 'Dark' });
  const before = await surface(page);
  await page
    .getByRole('textbox', { name: 'Import a theme' })
    .fill('{"id":"e2e-ink","name":"E2E Ink","base":"dark","tokens":{"surface":"#123456"}}');
  await page.getByRole('button', { name: 'Import', exact: true }).click();
  await expect(page.getByRole('combobox', { name: 'Theme preference' })).toContainText('E2E Ink', {
    timeout: 20_000,
  });
  await page.getByRole('combobox', { name: 'Theme preference' }).selectOption({ label: 'E2E Ink' });
  await expect.poll(() => surface(page), { timeout: 20_000 }).not.toBe(before);
  await page.getByRole('button', { name: 'Remove E2E Ink' }).click();
  await expect(page.getByRole('combobox', { name: 'Theme preference' })).not.toContainText('E2E Ink');
});

test('a theme that is not a theme is refused rather than applied', async ({ page }) => {
  await openSettings(page);
  await page.getByRole('textbox', { name: 'Import a theme' }).fill('{"nope":true}');
  await page.getByRole('button', { name: 'Import', exact: true }).click();
  await expect(page.getByRole('dialog', { name: 'Settings' })).toBeVisible();
  await expect(page.getByRole('combobox', { name: 'Theme preference' })).not.toContainText('nope');
});

test('the default log presentation survives a new document', async ({ page }) => {
  await openSection(page, 'Logs');
  let saved = settingsWrite(page);
  await page.getByLabel('Default log view').selectOption('raw');
  await saved;
  await page.reload();
  await page.getByRole('button', { name: 'Settings' }).click();
  await page.getByRole('button', { name: 'Logs', exact: true }).click();
  await expect(page.getByLabel('Default log view')).toHaveValue('raw');
  saved = settingsWrite(page);
  await page.getByLabel('Default log view').selectOption('pretty');
  await saved;
});

test('the audit refresh interval is stored by the server', async ({ page }) => {
  await openSection(page, 'Cluster');
  let saved = settingsWrite(page);
  await page.getByLabel('Check refresh interval').selectOption('15');
  await saved;
  await page.reload();
  await page.getByRole('button', { name: 'Settings' }).click();
  await page.getByRole('button', { name: 'Cluster', exact: true }).click();
  await expect(page.getByLabel('Check refresh interval')).toHaveValue('15');
  saved = settingsWrite(page);
  await page.getByLabel('Check refresh interval').selectOption('60');
  await saved;
});

test('the namespace start preference changes the live cluster scope', async ({ page }) => {
  await openSection(page, 'Cluster');
  let saved = settingsWrite(page);
  await page.getByLabel('Namespace to open on').selectOption('default');
  await saved;
  await expect(page.getByRole('combobox', { name: 'Namespace', exact: true })).toHaveValue(
    'default',
  );
  saved = settingsWrite(page);
  await page.getByLabel('Namespace to open on').selectOption('all');
  await saved;
  await expect(page.getByRole('combobox', { name: 'Namespace', exact: true })).toHaveValue('');
});

test('a custom column reaches the real resource table and can be removed', async ({ page }) => {
  await openSection(page, 'Columns');
  await page.getByLabel('Kind', { exact: true }).selectOption({ label: 'Pod' });
  await page.getByLabel('Column name').fill('E2E app');
  await page.getByLabel('Field path').fill('.metadata.labels.app');
  let saved = settingsWrite(page);
  await page.getByRole('button', { name: 'Add', exact: true }).click();
  await saved;
  await page.getByRole('button', { name: 'Close', exact: true }).click();
  await openResource(page, 'pods', 'Pod');
  await expect(page.getByRole('columnheader', { name: /E2E app/ })).toBeVisible({
    timeout: 60_000,
  });
  const healthy = page.locator('main tbody tr').filter({ hasText: 'healthy-' }).first();
  await expect(healthy).toContainText('healthy');
  await page.getByRole('button', { name: 'Settings' }).click();
  await page.getByRole('button', { name: 'Columns', exact: true }).click();
  await page.getByLabel('Kind', { exact: true }).selectOption({ label: 'Pod' });
  saved = settingsWrite(page);
  await page.getByRole('button', { name: 'Remove E2E app', exact: true }).click();
  await saved;
});

test('the keyboard section reports the shortcuts that work in the application', async ({ page }) => {
  await openSection(page, 'Keyboard');
  const table = page.getByRole('table', { name: 'Keyboard shortcuts' });
  await expect(table).toContainText('Open the command palette');
  await expect(table).toContainText('Jump to the resource filter');
  await expect(table).toContainText('Close the palette or dialog, then the inspector');
});

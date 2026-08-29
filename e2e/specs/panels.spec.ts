import { expect, test } from '../harness/test';
import { openHome, openResource, selectRow } from '../harness/app';
import type { Page } from '@playwright/test';

test.describe.configure({ mode: 'serial' });

async function openPod(page: Page): Promise<void> {
  await openResource(page, 'pods', 'Pod');
  await selectRow(page, 'healthy-');
  await expect(page.getByRole('tablist', { name: 'right panels' })).toBeVisible({
    timeout: 60_000,
  });
}

async function resetDocks(page: Page): Promise<void> {
  await openHome(page);
  const saved = page.waitForResponse(
    (response) =>
      response.url().includes('/api/settings') && response.request().method() === 'PUT',
    { timeout: 30_000 },
  );
  await page.getByRole('button', { name: 'Settings' }).click();
  await page
    .getByRole('navigation', { name: 'Settings sections' })
    .getByRole('button', { name: 'Panels', exact: true })
    .click();
  await page.getByRole('button', { name: 'Reset', exact: true }).click();
  await saved.catch(() => undefined);
  await page.keyboard.press('Escape');
}

test.beforeEach(async ({ page }) => {
  await resetDocks(page);
});

test.afterAll(async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();
  await resetDocks(page);
  await context.close();
});

test('each dock names itself and the panels it holds', async ({ page }) => {
  await openPod(page);
  const right = page.getByRole('tablist', { name: 'right panels' });
  await expect(right.getByRole('tab', { name: 'Overview', exact: true })).toBeVisible();
  const bottom = page.getByRole('tablist', { name: 'bottom panels' });
  await expect(bottom.getByRole('tab', { name: 'Forwards', exact: true })).toBeVisible();
  await expect(bottom.getByRole('tab', { name: 'Terminal', exact: true })).toBeVisible();
});

test('a dock hides and comes back', async ({ page }) => {
  await openPod(page);
  await page.getByRole('button', { name: 'Hide the right dock' }).click();
  await expect(page.getByRole('tablist', { name: 'right panels' })).toBeHidden();
  await page.getByRole('button', { name: 'Show the right dock' }).click();
  await expect(page.getByRole('tablist', { name: 'right panels' })).toBeVisible();
});

test('a panel moved to another dock lands there', async ({ page }) => {
  await openPod(page);
  await page.getByRole('tab', { name: 'Overview', exact: true }).click();
  await page.getByRole('button', { name: 'Move Overview to the bottom' }).click();
  await expect(
    page
      .getByRole('tablist', { name: 'bottom panels' })
      .getByRole('tab', { name: 'Overview', exact: true }),
  ).toBeVisible({ timeout: 20_000 });
  await expect(
    page
      .getByRole('tablist', { name: 'right panels' })
      .getByRole('tab', { name: 'Overview', exact: true }),
  ).toHaveCount(0);
});

test('the empty left dock takes a panel and stops being empty', async ({ page }) => {
  await openPod(page);
  await expect(page.getByRole('group', { name: 'Empty left dock' })).toBeVisible();
  await page.getByRole('tab', { name: 'YAML', exact: true }).click();
  await page.getByRole('button', { name: 'Move YAML to the left' }).click();
  await expect(
    page
      .getByRole('tablist', { name: 'left panels' })
      .getByRole('tab', { name: 'YAML', exact: true }),
  ).toBeVisible({ timeout: 20_000 });
});

test('where a panel was put survives a reload', async ({ page }) => {
  await openPod(page);
  await page.getByRole('tab', { name: 'Overview', exact: true }).click();
  const saved = page.waitForResponse(
    (response) =>
      response.url().includes('/api/settings') && response.request().method() === 'PUT',
    { timeout: 30_000 },
  );
  await page.getByRole('button', { name: 'Move Overview to the bottom' }).click();
  const moved = page
    .getByRole('tablist', { name: 'bottom panels' })
    .getByRole('tab', { name: 'Overview', exact: true });
  await expect(moved).toBeVisible({ timeout: 20_000 });
  await saved;
  await page.reload();
  await page.waitForLoadState('domcontentloaded');
  await expect(moved).toBeVisible({ timeout: 60_000 });
});

test('the dock layout can be put back the way it started', async ({ page }) => {
  await openPod(page);
  await page.getByRole('tab', { name: 'Overview', exact: true }).click();
  await page.getByRole('button', { name: 'Move Overview to the bottom' }).click();
  await expect(
    page
      .getByRole('tablist', { name: 'bottom panels' })
      .getByRole('tab', { name: 'Overview', exact: true }),
  ).toBeVisible({ timeout: 20_000 });
  await resetDocks(page);
  await openPod(page);
  await expect(
    page.getByRole('tablist', { name: 'right panels' }).getByRole('tab', { name: 'Overview', exact: true }),
  ).toBeVisible({ timeout: 30_000 });
});

test('the dock tabs move under the arrow keys', async ({ page }) => {
  await openPod(page);
  const overview = page.getByRole('tab', { name: 'Overview', exact: true });
  await overview.click();
  await overview.press('ArrowRight');
  await expect(page.getByRole('tab', { name: 'YAML', exact: true })).toBeFocused();
});

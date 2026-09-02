import { expect, test } from '../harness/test';
import { openHome, openResource, selectRow } from '../harness/app';
import type { Page, Response } from '@playwright/test';

function settingsWrite(key: string): (response: Response) => boolean {
  return (response) => {
    if (!response.url().includes('/api/settings')) {
      return false;
    }
    if (response.request().method() !== 'PUT') {
      return false;
    }
    const body = response.request().postData();
    if (body === null) {
      return false;
    }
    return body.includes(`"${key}"`);
  };
}

async function openPod(page: Page): Promise<void> {
  await openResource(page, 'pods', 'Pod');
  await selectRow(page, 'healthy-');
  await expect(page.getByRole('tablist', { name: 'right panels' })).toBeVisible({
    timeout: 60_000,
  });
}

async function resetDocks(page: Page): Promise<void> {
  await openHome(page);
  const saved = page.waitForResponse(settingsWrite('spinoza.layout.v1'), { timeout: 30_000 });
  await page.getByRole('button', { name: 'Settings' }).click();
  await page
    .getByRole('navigation', { name: 'Settings sections' })
    .getByRole('button', { name: 'Panels', exact: true })
    .click();
  await page.getByRole('button', { name: 'Reset', exact: true }).click();
  await saved;
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

test('the bottom dock collapses independently from the inspector', async ({ page }) => {
  await openPod(page);
  await page.getByRole('button', { name: 'Hide the bottom dock' }).click();
  await expect(page.getByRole('group', { name: 'Collapsed bottom dock' })).toBeVisible();
  await expect(page.getByRole('tablist', { name: 'bottom panels' })).toBeHidden();
  await expect(page.getByRole('tablist', { name: 'right panels' })).toBeVisible();
  await page.getByRole('button', { name: 'Show the bottom dock' }).click();
  await expect(page.getByRole('tablist', { name: 'bottom panels' })).toBeVisible();
});

test('a collapsed dock stays collapsed through a reload and can be restored', async ({ page }) => {
  await openPod(page);
  let saved = page.waitForResponse(settingsWrite('spinoza.layout.v1'), { timeout: 30_000 });
  await page.getByRole('button', { name: 'Hide the bottom dock' }).click();
  await saved;
  await page.reload();
  await expect(page.getByRole('group', { name: 'Collapsed bottom dock' })).toBeVisible({
    timeout: 60_000,
  });
  await expect(page.getByRole('tablist', { name: 'bottom panels' })).toBeHidden();
  saved = page.waitForResponse(settingsWrite('spinoza.layout.v1'), { timeout: 30_000 });
  await page.getByRole('button', { name: 'Show the bottom dock' }).click();
  await saved;
  await expect(page.getByRole('tablist', { name: 'bottom panels' })).toBeVisible();
});

test('the selected tab and its panel agree about what is open', async ({ page }) => {
  await openPod(page);
  const right = page.getByRole('tablist', { name: 'right panels' });
  const overview = right.getByRole('tab', { name: 'Overview', exact: true });
  const yaml = right.getByRole('tab', { name: 'YAML', exact: true });
  await overview.click();
  await expect(overview).toHaveAttribute('aria-selected', 'true');
  await expect(page.getByRole('tabpanel', { name: 'Overview' })).toBeVisible();
  await yaml.click();
  await expect(yaml).toHaveAttribute('aria-selected', 'true');
  await expect(overview).toHaveAttribute('aria-selected', 'false');
  await expect(page.getByRole('tabpanel', { name: 'YAML' })).toBeVisible();
  await expect(page.getByRole('tabpanel', { name: 'Overview' })).toBeHidden();
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
  const saved = page.waitForResponse(settingsWrite('spinoza.panels.v1'), { timeout: 30_000 });
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

test('closing the inspector clears the object route without leaving the table', async ({
  page,
}) => {
  await openPod(page);
  expect(page.url()).toContain('name=healthy-');
  await page
    .getByRole('tabpanel', { name: 'Overview' })
    .getByRole('button', { name: 'Close' })
    .click();
  await expect(page).not.toHaveTitle(/^healthy-/);
  await expect(page.locator('main tbody tr').filter({ hasText: 'healthy-' }).first()).toBeVisible();
  expect(page.url()).not.toContain('name=healthy-');
});

test('escape closes the selected object before it changes the view', async ({ page }) => {
  await openPod(page);
  const before = new URL(page.url()).hash;
  await page.keyboard.press('Escape');
  await expect(page).not.toHaveTitle(/^healthy-/);
  expect(new URL(page.url()).hash).not.toBe(before);
  await expect(page.locator('main tbody tr').filter({ hasText: 'healthy-' }).first()).toBeVisible();
});

test('the dock resize handle responds to keyboard input', async ({ page }) => {
  await openPod(page);
  const dock = page.getByRole('group', { name: 'right dock' });
  const before = await dock.boundingBox();
  expect(before).not.toBeNull();
  await page.getByRole('button', { name: 'Resize the right dock' }).press('ArrowLeft');
  const after = await dock.boundingBox();
  expect(after).not.toBeNull();
  if (before === null || after === null) {
    throw new Error('the right dock has no measurable box');
  }
  expect(after.width).not.toBe(before.width);
});

test('the keyboard-resized sidebar survives reload and the layout reset restores it', async ({
  page,
}) => {
  await openHome(page);
  const handle = page.getByRole('button', { name: 'Resize sidebar' });
  const sidebar = handle.locator('..');
  const before = await sidebar.boundingBox();
  expect(before).not.toBeNull();
  if (before === null) {
    throw new Error('the sidebar has no measurable box');
  }
  try {
    const saved = page.waitForResponse(settingsWrite('spinoza.layout.v1'), { timeout: 30_000 });
    await handle.press('ArrowRight');
    await saved;
    const resized = await sidebar.boundingBox();
    expect(resized).not.toBeNull();
    if (resized === null) {
      throw new Error('the resized sidebar has no measurable box');
    }
    expect(resized.width).toBeGreaterThan(before.width);
    await page.reload();
    await expect
      .poll(async () => (await sidebar.boundingBox())?.width, { timeout: 60_000 })
      .toBe(resized.width);
  } finally {
    await resetDocks(page);
  }
  await openHome(page);
  await expect.poll(async () => (await sidebar.boundingBox())?.width).toBe(before.width);
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
    page
      .getByRole('tablist', { name: 'right panels' })
      .getByRole('tab', { name: 'Overview', exact: true }),
  ).toBeVisible({ timeout: 30_000 });
});

test('the dock tabs move under the arrow keys', async ({ page }) => {
  await openPod(page);
  const overview = page.getByRole('tab', { name: 'Overview', exact: true });
  await overview.click();
  await overview.press('ArrowRight');
  await expect(page.getByRole('tab', { name: 'YAML', exact: true })).toBeFocused();
});

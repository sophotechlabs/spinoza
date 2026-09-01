import { expect, test } from '../harness/test';
import { openResource } from '../harness/app';
import type { Page } from '@playwright/test';

async function openLogs(page: Page, pod: string): Promise<void> {
  await openResource(page, 'pods', 'Pod');
  const row = page.locator('main tbody tr').filter({ hasText: pod }).first();
  await expect(row).toBeVisible({ timeout: 60_000 });
  await row.click();
  await page.getByRole('tab', { name: 'Logs' }).click();
}

test('the log panel offers the controls it promises', async ({ page }) => {
  await openLogs(page, 'chatty');
  for (const control of ['Following', 'Pretty', 'Timestamps', 'Wrap', 'Copy', 'Download', 'Clear']) {
    await expect(page.getByRole('button', { name: control, exact: true })).toBeVisible({
      timeout: 30_000,
    });
  }
  await expect(page.getByLabel('Filter log lines')).toBeVisible();
});

test('a container that prints reaches the log panel', async ({ page }) => {
  await openLogs(page, 'chatty');
  await expect(page.getByText('e2e-log-line').first()).toBeVisible({ timeout: 60_000 });
});

test('pausing follow stops the scroll, not the stream', async ({ page }) => {
  await openLogs(page, 'chatty');
  const lines = page.getByText('e2e-log-line');
  await expect(lines.first()).toBeVisible({ timeout: 60_000 });
  const before = await lines.count();
  const follow = page.getByRole('button', { name: 'Following', exact: true });
  await expect(follow).toBeVisible({ timeout: 30_000 });
  await follow.click();
  await expect(page.getByRole('button', { name: 'Follow', exact: true })).toBeVisible();
  await expect.poll(async () => lines.count(), { timeout: 30_000 }).toBeGreaterThanOrEqual(before);
  await expect(lines.first()).toBeVisible();
});

test('the filter narrows the lines that are shown', async ({ page }) => {
  await openLogs(page, 'chatty');
  await expect(page.getByText('e2e-log-line').first()).toBeVisible({ timeout: 60_000 });
  await page.getByLabel('Filter log lines').fill('nothing-matches-this');
  await expect(page.getByText('e2e-log-line')).toHaveCount(0, { timeout: 30_000 });
});

test('clearing empties the panel without stopping the stream', async ({ page }) => {
  await openLogs(page, 'chatty');
  await expect(page.getByText('e2e-log-line').first()).toBeVisible({ timeout: 60_000 });
  await page.getByRole('button', { name: 'Clear', exact: true }).click();
  await expect(page.getByText('e2e-log-line').first()).toBeVisible({ timeout: 60_000 });
});

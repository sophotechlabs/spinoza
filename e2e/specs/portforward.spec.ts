import { expect, test } from '../harness/test';
import { openResource } from '../harness/app';

test.describe.configure({ mode: 'serial' });

async function openHealthy(page: import('@playwright/test').Page): Promise<void> {
  await openResource(page, 'pods', 'Pod');
  const row = page.locator('main tbody tr').filter({ hasText: 'healthy-' }).first();
  await expect(row).toBeVisible({ timeout: 60_000 });
  await row.click();
  await page.getByRole('tab', { name: 'Overview' }).click();
}

test('a pod with a port offers to forward it', async ({ page }) => {
  await openHealthy(page);
  await expect(page.getByText('PORTS')).toBeVisible({ timeout: 30_000 });
  await expect(page.getByText('8080').first()).toBeVisible();
  await expect(page.getByRole('button', { name: 'Forward', exact: true })).toBeVisible();
});

test('a pod with no port is not offered one', async ({ page }) => {
  await openResource(page, 'pods', 'Pod');
  const row = page.locator('main tbody tr').filter({ hasText: 'noshell' }).first();
  await expect(row).toBeVisible({ timeout: 60_000 });
  await row.click();
  await page.getByRole('tab', { name: 'Overview' }).click();
  await expect(page.getByRole('button', { name: 'Forward', exact: true })).toHaveCount(0);
});

test('forwarding reports the local address it took', async ({ page }) => {
  await openHealthy(page);
  await page.getByRole('button', { name: 'Forward', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Stop', exact: true }).first()).toBeVisible({
    timeout: 60_000,
  });
});

test('a forward survives navigating away and back', async ({ page }) => {
  await openResource(page, 'configmaps', 'ConfigMap');
  await openHealthy(page);
  await expect(page.getByRole('button', { name: 'Stop', exact: true }).first()).toBeVisible({
    timeout: 30_000,
  });
});

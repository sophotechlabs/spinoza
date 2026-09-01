import { expect, test } from '../harness/test';
import { openResource } from '../harness/app';

async function openHealthy(page: import('@playwright/test').Page): Promise<void> {
  await openResource(page, 'pods', 'Pod');
  const row = page
    .locator('main tbody tr')
    .filter({ hasText: 'healthy-' })
    .filter({ hasText: 'Running' })
    .first();
  await expect(row).toBeVisible({ timeout: 90_000 });
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
  await page.getByRole('tab', { name: 'Forwards', exact: true }).click();
  const forwards = page.getByRole('tabpanel', { name: 'Forwards' });
  await expect(forwards).toContainText('healthy-', { timeout: 60_000 });
  await expect(forwards).toContainText(/127\.0\.0\.1:\d+|localhost:\d+|:\d{4,5}/);
  await expect(forwards.getByRole('button', { name: 'Stop', exact: true }).first()).toBeVisible({
    timeout: 60_000,
  });
});

test('a forward survives navigating away and back', async ({ page }) => {
  await openResource(page, 'configmaps', 'ConfigMap');
  await openHealthy(page);
  await page.getByRole('tab', { name: 'Forwards', exact: true }).click();
  const forwards = page.getByRole('tabpanel', { name: 'Forwards' });
  await expect(forwards.getByRole('button', { name: 'Stop', exact: true }).first()).toBeVisible({
    timeout: 30_000,
  });
  await forwards.getByRole('button', { name: 'Stop', exact: true }).first().click();
  await expect(forwards).toContainText('No active forwards', { timeout: 30_000 });
});

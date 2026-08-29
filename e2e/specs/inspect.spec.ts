import { expect, test } from '../harness/test';
import { ensureDrawer, openResource } from '../harness/app';
import type { Page } from '@playwright/test';

async function open(page: Page, resource: string, kind: string, name: string): Promise<void> {
  await openResource(page, resource, kind);
  const row = page.locator('main tbody tr').filter({ hasText: name }).first();
  await expect(row).toBeVisible({ timeout: 60_000 });
  await row.click();
  await ensureDrawer(page);
}

test('the drawer reports metadata, conditions and containers', async ({ page }) => {
  await open(page, 'pods', 'Pod', 'noshell');
  await page.getByRole('tab', { name: 'Overview' }).click();
  for (const section of ['METADATA', 'CONDITIONS', 'CONTAINERS', 'LABELS']) {
    await expect(page.getByText(section, { exact: true }).first()).toBeVisible({ timeout: 30_000 });
  }
});

test('the drawer names the owner that made the object', async ({ page }) => {
  await open(page, 'pods', 'Pod', 'chatty-');
  await page.getByRole('tab', { name: 'Overview' }).click();
  await expect(page.getByText('OWNER REFERENCES')).toBeVisible({ timeout: 30_000 });
  await expect(page.getByText('ReplicaSet').first()).toBeVisible();
});

test('the yaml tab shows the live object and offers to apply it', async ({ page }) => {
  await open(page, 'pods', 'Pod', 'noshell');
  await page.getByRole('tab', { name: 'YAML' }).click();
  await expect(page.getByRole('button', { name: 'Apply', exact: true })).toBeVisible({
    timeout: 60_000,
  });
  await expect(page.getByRole('button', { name: 'Delete', exact: true })).toBeVisible();
});

test('the events tab reports what the cluster said about a failing pod', async ({ page }) => {
  await open(page, 'pods', 'Pod', 'crashing-');
  await page.getByRole('tab', { name: 'Events' }).click();
  await expect(page.getByText(/BackOff|Failed|Unhealthy/).first()).toBeVisible({ timeout: 60_000 });
});

test('a secret opens in the drawer without spilling its value', async ({ page }) => {
  await open(page, 'secrets', 'Secret', 'secret-sample');
  await expect(page).toHaveTitle(/^secret-sample /);
  await expect(page.locator('body')).not.toContainText('e2e-password');
});

test('the drawer follows the selection into the title', async ({ page }) => {
  await open(page, 'pods', 'Pod', 'noshell');
  await expect(page).toHaveTitle(/^noshell pods /);
  await open(page, 'pods', 'Pod', 'chatty-');
  await expect(page).toHaveTitle(/^chatty-/);
});

import { expect, test } from '../harness/test';
import { openGrouped, openHome, openResource, openView, sidebar } from '../harness/app';
import { SCALE_CONFIGMAPS, SCALE_NAMESPACE, SCALE_WORKLOADS } from '../harness/fixtures';
import type { Page } from '@playwright/test';

async function scopeTo(page: Page, namespace: string): Promise<void> {
  await page.getByRole('combobox', { name: 'Namespace' }).selectOption({ label: namespace });
}

test('a table of thousands of rows still renders a window of them', async ({ page }) => {
  await openResource(page, 'configmaps', 'ConfigMap');
  await scopeTo(page, SCALE_NAMESPACE);
  await expect(page.locator('main')).toContainText(String(SCALE_CONFIGMAPS), { timeout: 90_000 });
  const rendered = await page.locator('main tbody tr').count();
  expect(rendered).toBeGreaterThan(0);
  expect(rendered).toBeLessThan(SCALE_CONFIGMAPS);
});

test('scrolling a virtualised table reaches rows that were never rendered', async ({ page }) => {
  await openResource(page, 'configmaps', 'ConfigMap');
  await scopeTo(page, SCALE_NAMESPACE);
  const rows = page.locator('main tbody tr');
  await expect(rows.first()).toBeVisible({ timeout: 90_000 });
  const last = `bulk-${String(SCALE_CONFIGMAPS - 1).padStart(4, '0')}`;
  await expect(rows.filter({ hasText: last })).toHaveCount(0);
  await page.getByPlaceholder('Filter by name, or field:value').fill(last);
  await expect(rows.filter({ hasText: last }).first()).toBeVisible({ timeout: 60_000 });
});

test('the header survives scrolling through the whole table', async ({ page }) => {
  await openResource(page, 'configmaps', 'ConfigMap');
  await scopeTo(page, SCALE_NAMESPACE);
  await expect(page.locator('main tbody tr').first()).toBeVisible({ timeout: 90_000 });
  await page.mouse.move(800, 500);
  for (let turn = 0; turn < 40; turn += 1) {
    await page.mouse.wheel(0, 2000);
  }
  await expect(page.locator('main thead th').filter({ hasText: 'Name' }).first()).toBeVisible();
  await expect(page.locator('main tbody tr').first()).toBeVisible();
});

test('the sidebar counts every type inside its budget', async ({ page }) => {
  await openHome(page);
  await expect(sidebar(page, /^Config \d+$/)).toBeVisible({ timeout: 120_000 });
  await expect(sidebar(page, /^Workloads \d+$/)).toBeVisible();
});

test('the checks payload stays bounded when the cluster is not', async ({ page }) => {
  await openView(page, 'checks');
  const main = page.locator('main');
  await expect(main).toContainText(/\d+ findings across \d+ workloads/, { timeout: 180_000 });
  await expect(main).toContainText('Security');
});

test('the issue queue stays inside its row cap on a busy cluster', async ({ page }) => {
  await openView(page, 'issues');
  await expect(page.locator('main')).toContainText(/broken|degraded|warning/, { timeout: 180_000 });
  expect(await page.locator('main tbody tr').count()).toBeLessThan(SCALE_WORKLOADS);
});

test('a workload table with hundreds of rows still filters to one', async ({ page }) => {
  await openGrouped(page, 'apps', 'deployments', 'Deployment');
  await scopeTo(page, SCALE_NAMESPACE);
  const rows = page.locator('main tbody tr');
  await expect(rows.first()).toBeVisible({ timeout: 90_000 });
  await page.getByPlaceholder('Filter by name, or field:value').fill('idle-0299');
  await expect(rows).toHaveCount(1, { timeout: 60_000 });
});

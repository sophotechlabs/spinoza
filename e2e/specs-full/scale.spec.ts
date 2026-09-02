import { expect, test } from '../harness/test';
import { openGrouped, openHome, openResource, openView, sidebar } from '../harness/app';
import { SCALE_CONFIGMAPS, SCALE_WORKLOADS } from '../harness/fixtures';
import type { Page } from '@playwright/test';

test.describe.configure({ timeout: 300_000 });

const FILTER = 'Filter by name, or field:value';

async function counted(page: Page): Promise<number> {
  const text = await page.locator('main').innerText();
  const found = /(\d+) of (\d+)/.exec(text);
  if (found === null) {
    return 0;
  }
  return Number(found[2]);
}

async function loaded(page: Page): Promise<number> {
  const text = await page.locator('main').innerText();
  const found = /(\d+) of (\d+)/.exec(text);
  if (found === null) {
    return 0;
  }
  return Number(found[1]);
}

test('a table of thousands of rows renders only a window of them', async ({ page }) => {
  await openResource(page, 'configmaps', 'ConfigMap');
  await expect.poll(() => counted(page), { timeout: 180_000 }).toBeGreaterThan(SCALE_CONFIGMAPS);
  const rendered = await page.locator('main tbody tr').count();
  expect(rendered).toBeGreaterThan(0);
  expect(rendered).toBeLessThan(SCALE_CONFIGMAPS);
});

test('the filter reaches a row the table never rendered', async ({ page }) => {
  await openResource(page, 'configmaps', 'ConfigMap');
  const rows = page.locator('main tbody tr');
  await expect(rows.first()).toBeVisible({ timeout: 180_000 });
  const last = `bulk-${String(SCALE_CONFIGMAPS - 1).padStart(4, '0')}`;
  await expect(rows.filter({ hasText: last })).toHaveCount(0);
  await page.getByPlaceholder(FILTER).fill(last);
  await expect(rows.filter({ hasText: last }).first()).toBeVisible({ timeout: 60_000 });
});

test('loading the next page increases the live subscription without rendering it all', async ({
  page,
}) => {
  await openResource(page, 'events', 'Event');
  const more = page.getByRole('button', { name: 'Load 100 more' });
  await expect(more).toBeVisible({ timeout: 180_000 });
  const before = await loaded(page);
  const total = await counted(page);
  expect(before).toBeGreaterThan(0);
  expect(before).toBeLessThan(total);
  await more.click();
  const expected = Math.min(before + 100, total);
  await expect.poll(() => loaded(page), { timeout: 120_000 }).toBe(expected);
  expect(await page.locator('main tbody tr').count()).toBeLessThan(expected);
});

test('the header and the rows survive scrolling the whole table', async ({ page }) => {
  await openResource(page, 'configmaps', 'ConfigMap');
  await expect(page.locator('main tbody tr').first()).toBeVisible({ timeout: 180_000 });
  await page.mouse.move(800, 500);
  for (let turn = 0; turn < 40; turn += 1) {
    await page.mouse.wheel(0, 2000);
  }
  await expect(page.locator('main thead th').filter({ hasText: 'Name' }).first()).toBeVisible();
  await expect(page.locator('main tbody tr').first()).toBeVisible();
});

test('the sidebar counts every type on a cluster this size', async ({ page }) => {
  await openHome(page);
  await expect(sidebar(page, /^Config \d+$/)).toBeVisible({ timeout: 180_000 });
  await expect(sidebar(page, /^Workloads \d+$/)).toBeVisible();
});

test('the checks payload stays bounded when the cluster is not', async ({ page }) => {
  await openView(page, 'checks');
  const main = page.locator('main');
  await expect(main).toContainText(/\d+ findings across \d+ workloads/, { timeout: 240_000 });
  await expect(main).toContainText('Security');
});

test('the issue queue stays inside its row cap on a busy cluster', async ({ page }) => {
  await openView(page, 'issues');
  await expect(page.locator('main')).toContainText(/broken|degraded|warning/, { timeout: 240_000 });
  expect(await page.locator('main tbody tr').count()).toBeLessThan(SCALE_WORKLOADS);
});

test('a workload table with hundreds of rows still filters to one', async ({ page }) => {
  await openGrouped(page, 'apps', 'deployments', 'Deployment');
  const rows = page.locator('main tbody tr');
  await expect(rows.first()).toBeVisible({ timeout: 180_000 });
  await page.getByPlaceholder(FILTER).fill('idle-0299');
  await expect(rows).toHaveCount(1, { timeout: 60_000 });
});

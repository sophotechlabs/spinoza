import { expect, test } from '../harness/test';
import { openHome, openResource, selectRow } from '../harness/app';
import type { Page } from '@playwright/test';

async function openMetrics(page: Page): Promise<void> {
  await openResource(page, 'pods', 'Pod');
  await selectRow(page, 'chatty-');
  await page.getByRole('tab', { name: 'Metrics', exact: true }).click();
  await expect(page.getByRole('tabpanel', { name: 'Metrics' })).toBeVisible({ timeout: 30_000 });
}

test('the overview reports live usage once metrics-server answers', async ({ page }) => {
  await openHome(page);
  const overview = page.getByRole('group', { name: 'Cluster overview' });
  await expect(overview).toContainText('Allocatable capacity', { timeout: 60_000 });
  await expect(overview).not.toContainText('Live usage needs metrics-server', { timeout: 60_000 });
});

test('the metrics panel says where its numbers come from', async ({ page }) => {
  await openMetrics(page);
  await expect(page.getByRole('tabpanel', { name: 'Metrics' })).toContainText(
    'Spinoza is measuring this itself: it found no Prometheus to ask.',
    { timeout: 30_000 },
  );
});

test('the range is offered, and the chart is drawn rather than described', async ({ page }) => {
  await openMetrics(page);
  const panel = page.getByRole('tabpanel', { name: 'Metrics' });
  await expect(panel.getByRole('combobox', { name: 'Metric range' })).toBeVisible({
    timeout: 30_000,
  });
  await expect(panel.locator('canvas').first()).toBeVisible({ timeout: 30_000 });
  expect(await panel.locator('canvas').count()).toBeGreaterThan(1);
});

test('the samples it takes itself turn into points on the chart', async ({ page }) => {
  await openMetrics(page);
  const panel = page.getByRole('tabpanel', { name: 'Metrics' });
  await expect(panel).not.toContainText('Nothing measured yet', { timeout: 120_000 });
  await expect(panel.locator('canvas').first()).toBeVisible({ timeout: 30_000 });
});

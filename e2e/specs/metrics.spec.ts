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

test('self-sampled history only offers ranges it can actually retain', async ({ page }) => {
  await openMetrics(page);
  const panel = page.getByRole('tabpanel', { name: 'Metrics' });
  await expect(panel).toContainText('Spinoza is measuring this itself', { timeout: 60_000 });
  const range = panel.getByRole('combobox', { name: 'Metric range' });
  await expect(range.getByRole('option')).toHaveText(['15m', '1h']);
  await expect(range).toHaveValue('1h');
});

test('changing the metric range changes the history request', async ({ page }) => {
  await openMetrics(page);
  const range = page.getByRole('tabpanel', { name: 'Metrics' }).getByRole('combobox', {
    name: 'Metric range',
  });
  const request = page.waitForRequest((candidate) => {
    const url = new URL(candidate.url());
    return url.pathname === '/api/metrics/history' && url.searchParams.get('range') === '15m';
  });
  await range.selectOption('15m');
  await request;
  await expect(range).toHaveValue('15m');
});

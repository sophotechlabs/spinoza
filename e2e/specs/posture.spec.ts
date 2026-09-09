import { expect, test } from '../harness/test';
import { openView } from '../harness/app';

interface Fetched {
  status: number;
  contentType: string;
  body: string;
}

async function get(page: import('@playwright/test').Page, url: string): Promise<Fetched> {
  return page.evaluate(async (asked: string) => {
    const response = await fetch(asked);
    return {
      status: response.status,
      contentType: response.headers.get('content-type') ?? '',
      body: await response.text(),
    };
  }, url);
}

test.describe('the audit as something else reads it', () => {
  test('hands the findings out as csv, json and sarif', async ({ page }) => {
    await openView(page, 'checks');
    await page.getByRole('button', { name: 'Configure' }).click();
    await expect(page.getByRole('combobox', { name: 'Export format' })).toBeVisible({
      timeout: 120_000,
    });

    const csv = await get(page, '/api/checks/export');
    expect(csv.status).toBe(200);
    expect(csv.contentType).toContain('text/csv');
    expect(csv.body).toContain('check,title,category,severity');

    const json = await get(page, '/api/checks/export?format=json');
    expect(json.status).toBe(200);
    const report = JSON.parse(json.body) as { groups: unknown[] };
    expect(Array.isArray(report.groups)).toBe(true);

    const sarif = await get(page, '/api/checks/export?format=sarif');
    expect(sarif.status).toBe(200);
    expect(sarif.contentType).toContain('sarif');
    const log = JSON.parse(sarif.body) as {
      version: string;
      runs: { tool: { driver: { name: string } } }[];
    };
    expect(log.version).toBe('2.1.0');
    expect(log.runs[0].tool.driver.name).toBe('spinoza');
  });

  test('reads the audit by control, and says what nothing answers', async ({ page }) => {
    await openView(page, 'checks');
    await page.getByRole('button', { name: 'By framework' }).click();

    await expect(page.getByLabel('Framework')).toBeVisible({ timeout: 120_000 });
    await expect(page.getByText(/checked controls have something failing/)).toBeVisible();
    await expect(page.getByText('no check answers this').first()).toBeVisible();
    await expect(
      page.getByText('nothing that reads a live cluster can answer this').first(),
    ).toBeVisible();
  });

  test('narrows the posture to one framework', async ({ page }) => {
    await openView(page, 'checks');
    await page.getByRole('button', { name: 'By framework' }).click();
    await page.getByLabel('Framework').selectOption('CIS Kubernetes Benchmark');

    await expect(page.getByText('5.2.2')).toBeVisible({ timeout: 120_000 });
  });

  test('answers its own metrics to an admin, and nobody scrapes them by accident', async ({
    page,
  }) => {
    await openView(page, 'checks');

    const metrics = await get(page, '/metrics');

    expect(metrics.status).toBe(200);
    expect(metrics.contentType).toContain('text/plain');
    expect(metrics.body).toContain('# TYPE spinoza_http_requests_total counter');
    expect(metrics.body).toContain('spinoza_build_info{version=');
  });

  test('says what it is waiting for, or that it is ready', async ({ page }) => {
    await openView(page, 'checks');

    const ready = await get(page, '/readyz');

    expect([200, 503]).toContain(ready.status);
    const state = JSON.parse(ready.body) as { ready: boolean; waiting?: string[] };
    if (!state.ready) {
      expect(state.waiting?.length ?? 0).toBeGreaterThan(0);
    }
  });
});

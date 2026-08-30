import { expect, test } from '../harness/test';
import { openHome } from '../harness/app';
import { CONTEXT, NOWHERE_CONTEXT, NOWHERE_KUBECONFIG } from '../harness/paths';
import type { Page } from '@playwright/test';

test.describe.configure({ mode: 'serial' });

interface Opened {
  id: string;
  context: string;
  active: boolean;
  reachable: boolean;
  color: number;
  protection: string;
}

async function opened(page: Page): Promise<Opened[]> {
  return page.evaluate(async () => {
    const response = await fetch('/api/clusters');
    const body = (await response.json()) as { clusters: unknown[] };
    return body.clusters as never;
  });
}

async function addSecond(page: Page): Promise<void> {
  await page.evaluate(async (path) => {
    await fetch(`/api/kubeconfigs?path=${encodeURIComponent(path)}`, { method: 'POST' });
  }, NOWHERE_KUBECONFIG);
}

async function openSecond(page: Page): Promise<{ status: number; body: string }> {
  return page.evaluate(
    async ([path, name]) => {
      const query = `name=${encodeURIComponent(name)}&kubeconfig=${encodeURIComponent(path)}`;
      const response = await fetch(`/api/clusters?${query}`, { method: 'POST' });
      return { status: response.status, body: await response.text() };
    },
    [NOWHERE_KUBECONFIG, NOWHERE_CONTEXT],
  );
}

test.afterAll(async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();
  await openHome(page);
  await page.evaluate(async (path) => {
    await fetch(`/api/kubeconfigs?path=${encodeURIComponent(path)}`, { method: 'DELETE' });
  }, NOWHERE_KUBECONFIG);
  await context.close();
});

test('one cluster is open, and it is the one spinoza was pointed at', async ({ page }) => {
  await openHome(page);
  const clusters = await opened(page);
  expect(clusters).toHaveLength(1);
  expect(clusters[0].context).toBe(CONTEXT);
  expect(clusters[0].active).toBe(true);
  expect(clusters[0].reachable).toBe(true);
});

test('the picker names the cluster and the file it came from', async ({ page }) => {
  await openHome(page);
  await page.locator('header').getByRole('group').first().click();
  const picker = page.locator('header').getByRole('group').first();
  await expect(picker).toContainText(CONTEXT, { timeout: 30_000 });
  await expect(picker.getByRole('button', { name: 'Manage kubeconfigs' })).toBeVisible();
});

test('a second kubeconfig puts its context in the picker', async ({ page }) => {
  await openHome(page);
  await addSecond(page);
  await page.reload();
  await page.waitForLoadState('domcontentloaded');
  await page.locator('header').getByRole('group').first().click();
  await expect(
    page.locator('header').getByRole('button', { name: NOWHERE_CONTEXT, exact: true }),
  ).toBeVisible({ timeout: 30_000 });
});

test('a cluster that never answers is refused, not added as a broken one', async ({ page }) => {
  await openHome(page);
  const refused = await openSecond(page);
  expect(refused.status).toBe(503);
  expect(refused.body).not.toBe('');

  await expect.poll(async () => (await opened(page)).length, { timeout: 30_000 }).toBe(1);
  expect((await opened(page))[0].context).toBe(CONTEXT);
});

test('the cluster that is open carries a colour and a protection state', async ({ page }) => {
  await openHome(page);
  const clusters = await opened(page);
  expect(clusters).toHaveLength(1);
  expect(clusters[0].color).toBeGreaterThan(0);
  expect(clusters[0].protection).toBe('open');
});

test('the cluster that answers keeps serving after the refusal', async ({ page }) => {
  await openHome(page);
  await expect(page.getByRole('status', { name: 'The cluster feed is connected' })).toBeVisible({
    timeout: 60_000,
  });
  await expect(page.getByRole('group', { name: 'Cluster overview' })).toContainText(/\d+ ready/, {
    timeout: 60_000,
  });
});

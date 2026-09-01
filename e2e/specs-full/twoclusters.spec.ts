import { expect, test } from '../harness/test';
import { openHome } from '../harness/app';
import { CONTEXT, SECOND_CONTEXT, SECOND_KUBECONFIG } from '../harness/paths';
import { kubectl } from '../harness/cluster';
import type { Page } from '@playwright/test';

interface Opened {
  id: string;
  context: string;
  active: boolean;
  reachable: boolean;
  color: number;
}

async function opened(page: Page): Promise<Opened[]> {
  return page.evaluate(async () => {
    const response = await fetch('/api/clusters');
    const body = (await response.json()) as { clusters: unknown[] };
    return body.clusters as never;
  });
}

async function openSecond(page: Page): Promise<number> {
  return page.evaluate(
    async ([path, name]) => {
      await fetch(`/api/kubeconfigs?path=${encodeURIComponent(path)}`, { method: 'POST' });
      const query = `name=${encodeURIComponent(name)}&kubeconfig=${encodeURIComponent(path)}`;
      const response = await fetch(`/api/clusters?${query}`, { method: 'POST' });
      return response.status;
    },
    [SECOND_KUBECONFIG, SECOND_CONTEXT],
  );
}

async function activate(page: Page, id: string): Promise<void> {
  await page.evaluate(async (cluster) => {
    await fetch(`/api/clusters/active?cluster=${encodeURIComponent(cluster)}`, { method: 'POST' });
  }, id);
}

function idOf(clusters: Opened[], context: string): string {
  const found = clusters.find((one) => one.context === context);
  if (found === undefined) {
    throw new Error(`no open cluster for ${context}`);
  }
  return found.id;
}

test('a second cluster that answers is opened and kept', async ({ page }) => {
  await openHome(page);
  expect(await openSecond(page)).toBe(200);
  await expect.poll(async () => (await opened(page)).length, { timeout: 60_000 }).toBe(2);
});

test('both clusters report themselves reachable, each with its own colour', async ({ page }) => {
  await openHome(page);
  const clusters = await opened(page);
  expect(clusters).toHaveLength(2);
  for (const one of clusters) {
    expect(one.reachable).toBe(true);
  }
  expect(new Set(clusters.map((one) => one.color)).size).toBe(2);
});

test('only one cluster is active at a time', async ({ page }) => {
  await openHome(page);
  const clusters = await opened(page);
  expect(clusters.filter((one) => one.active)).toHaveLength(1);
});

test('activating the second cluster moves the app onto it', async ({ page }) => {
  await openHome(page);
  await activate(page, idOf(await opened(page), SECOND_CONTEXT));

  await expect
    .poll(
      async () => (await opened(page)).find((one) => one.active)?.context,
      { timeout: 60_000 },
    )
    .toBe(SECOND_CONTEXT);
  await page.reload();
  await page.waitForLoadState('domcontentloaded');
  await expect(page.getByRole('banner')).toContainText(SECOND_CONTEXT, { timeout: 60_000 });
});

test('the second cluster is read, not the first one over again', async ({ page }) => {
  await openHome(page);
  const nodes = kubectl(['get', 'nodes', '-o', 'jsonpath={.items[*].metadata.name}']).split(' ');
  expect(nodes.length).toBeGreaterThan(1);
  await expect(page.getByRole('group', { name: 'Cluster overview' })).toContainText('1 ready', {
    timeout: 90_000,
  });
});

test('going back to the first cluster brings its own nodes with it', async ({ page }) => {
  await openHome(page);
  await activate(page, idOf(await opened(page), CONTEXT));
  await expect
    .poll(
      async () => (await opened(page)).find((one) => one.active)?.context,
      { timeout: 60_000 },
    )
    .toBe(CONTEXT);
  await page.reload();
  await page.waitForLoadState('domcontentloaded');
  await expect(page.getByRole('group', { name: 'Cluster overview' })).toContainText('6 ready', {
    timeout: 90_000,
  });
});

test('closing the second cluster leaves the first one serving', async ({ page }) => {
  await openHome(page);
  await page.evaluate(async (id) => {
    await fetch(`/api/clusters?cluster=${encodeURIComponent(id)}`, { method: 'DELETE' });
  }, idOf(await opened(page), SECOND_CONTEXT));

  await expect.poll(async () => (await opened(page)).length, { timeout: 60_000 }).toBe(1);
  expect((await opened(page))[0].context).toBe(CONTEXT);
  await expect(page.getByRole('status', { name: 'The cluster feed is connected' })).toBeVisible({
    timeout: 60_000,
  });
});

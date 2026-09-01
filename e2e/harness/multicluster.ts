import type { Page } from '@playwright/test';
import { expect } from '@playwright/test';
import { CONTEXT, SECOND_CONTEXT, SECOND_KUBECONFIG } from './paths';

export interface OpenedCluster {
  id: string;
  context: string;
  active: boolean;
  reachable: boolean;
  color: number;
}

export async function openedClusters(page: Page): Promise<OpenedCluster[]> {
  return page.evaluate(async () => {
    const response = await fetch('/api/clusters');
    const body = (await response.json()) as { clusters: unknown[] };
    return body.clusters as never;
  });
}

export async function openSecondCluster(page: Page): Promise<void> {
  const held = await openedClusters(page);
  if (held.some((cluster) => cluster.context === SECOND_CONTEXT)) {
    return;
  }
  const status = await page.evaluate(
    async ([path, name]) => {
      const loaded = await fetch(`/api/kubeconfigs?path=${encodeURIComponent(path)}`, {
        method: 'POST',
      });
      if (!loaded.ok) {
        return loaded.status;
      }
      const query = `name=${encodeURIComponent(name)}&kubeconfig=${encodeURIComponent(path)}`;
      const response = await fetch(`/api/clusters?${query}`, { method: 'POST' });
      return response.status;
    },
    [SECOND_KUBECONFIG, SECOND_CONTEXT],
  );
  expect(status).toBe(200);
  await expect
    .poll(
      async () => (await openedClusters(page)).some((cluster) => cluster.context === SECOND_CONTEXT),
      { timeout: 90_000 },
    )
    .toBe(true);
}

export async function activateContext(page: Page, context: string): Promise<void> {
  const cluster = clusterID(await openedClusters(page), context);
  const status = await page.evaluate(async (id) => {
    const response = await fetch(`/api/clusters/active?cluster=${encodeURIComponent(id)}`, {
      method: 'POST',
    });
    return response.status;
  }, cluster);
  expect(status).toBe(200);
  await expect
    .poll(
      async () => (await openedClusters(page)).find((entry) => entry.active)?.context,
      { timeout: 90_000 },
    )
    .toBe(context);
}

export function clusterID(clusters: OpenedCluster[], context: string): string {
  const found = clusters.find((cluster) => cluster.context === context);
  if (found === undefined) {
    throw new Error(`no open cluster for ${context}`);
  }
  return found.id;
}

export async function ensurePrimaryActive(page: Page): Promise<void> {
  await activateContext(page, CONTEXT);
}

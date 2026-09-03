import { expect, test } from '../harness/test';
import { openHome, openPalette } from '../harness/app';
import { CONTEXT, SECOND_CONTEXT, SECOND_KUBECONFIG } from '../harness/paths';
import { kubectl, kubectlSecond } from '../harness/cluster';
import type { Page } from '@playwright/test';

interface Opened {
  id: string;
  context: string;
  active: boolean;
  reachable: boolean;
  color: number;
  label?: string;
  grouping?: string;
  reopen: boolean;
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

async function ensureSecond(page: Page): Promise<void> {
  const clusters = await opened(page);
  if (clusters.some((one) => one.context === SECOND_CONTEXT)) {
    return;
  }
  expect(await openSecond(page)).toBe(200);
  await expect
    .poll(async () => (await opened(page)).some((one) => one.context === SECOND_CONTEXT), {
      timeout: 60_000,
    })
    .toBe(true);
}

async function activate(page: Page, id: string): Promise<void> {
  const cluster = (await opened(page)).find((one) => one.id === id);
  if (cluster === undefined) {
    throw new Error(`no open cluster for ${id}`);
  }
  const picker = page.getByLabel('Kubernetes context');
  await picker.click();
  await page.locator('header').getByRole('button', { name: cluster.context, exact: true }).click();
  await expect
    .poll(async () => (await opened(page)).find((one) => one.active)?.id, { timeout: 60_000 })
    .toBe(id);
  let shown = cluster.context;
  if (cluster.label !== undefined && cluster.label !== '') {
    shown = cluster.label;
  }
  await expect(picker).toContainText(shown, { timeout: 60_000 });
}

async function changeCluster(
  page: Page,
  route: string,
  values: Record<string, string>,
): Promise<number> {
  return page.evaluate(
    async ([path, fields]) => {
      const query = new URLSearchParams(fields);
      const response = await fetch(`/api/clusters/${path}?${query.toString()}`, {
        method: 'POST',
      });
      return response.status;
    },
    [route, values] as const,
  );
}

function idOf(clusters: Opened[], context: string): string {
  const found = clusters.find((one) => one.context === context);
  if (found === undefined) {
    throw new Error(`no open cluster for ${context}`);
  }
  return found.id;
}

async function openSettings(page: Page, name: string): Promise<void> {
  await page
    .getByRole('button', { name: `${name} is answering; open its settings`, exact: true })
    .click();
  await expect(page.getByRole('group', { name: `Settings for ${name}` })).toBeVisible();
}

test('a second cluster that answers is opened and kept', async ({ page }) => {
  await openHome(page);
  expect(await openSecond(page)).toBe(200);
  await expect.poll(async () => (await opened(page)).length, { timeout: 60_000 }).toBe(2);
});

test('both clusters report themselves reachable, each with its own colour', async ({ page }) => {
  await openHome(page);
  await ensureSecond(page);
  const clusters = await opened(page);
  expect(clusters).toHaveLength(2);
  for (const one of clusters) {
    expect(one.reachable).toBe(true);
  }
  expect(new Set(clusters.map((one) => one.color)).size).toBe(2);
});

test('only one cluster is active at a time', async ({ page }) => {
  await openHome(page);
  await ensureSecond(page);
  const clusters = await opened(page);
  expect(clusters.filter((one) => one.active)).toHaveLength(1);
});

test('activating the second cluster moves the app onto it', async ({ page }) => {
  await openHome(page);
  await ensureSecond(page);
  await activate(page, idOf(await opened(page), SECOND_CONTEXT));

  await expect
    .poll(async () => (await opened(page)).find((one) => one.active)?.context, { timeout: 60_000 })
    .toBe(SECOND_CONTEXT);
  await page.reload();
  await page.waitForLoadState('domcontentloaded');
  await expect(page.getByRole('banner')).toContainText(SECOND_CONTEXT, { timeout: 60_000 });
});

test('the second cluster is read, not the first one over again', async ({ page }) => {
  await openHome(page);
  await ensureSecond(page);
  await activate(page, idOf(await opened(page), SECOND_CONTEXT));
  await expect
    .poll(async () => (await opened(page)).find((one) => one.active)?.context, { timeout: 60_000 })
    .toBe(SECOND_CONTEXT);
  await page.reload();
  await page.waitForLoadState('domcontentloaded');
  const primary = JSON.parse(kubectl(['get', 'nodes', '-o', 'json'])) as { items: unknown[] };
  const secondary = JSON.parse(kubectlSecond(['get', 'nodes', '-o', 'json'])) as {
    items: unknown[];
  };
  expect(primary.items.length).not.toBe(secondary.items.length);
  await expect(page.getByRole('group', { name: 'Cluster overview' })).toContainText(
    `${String(secondary.items.length)} ready`,
    { timeout: 90_000 },
  );
});

test('going back to the first cluster brings its own nodes with it', async ({ page }) => {
  await openHome(page);
  await ensureSecond(page);
  await activate(page, idOf(await opened(page), CONTEXT));
  await expect
    .poll(async () => (await opened(page)).find((one) => one.active)?.context, { timeout: 60_000 })
    .toBe(CONTEXT);
  await page.reload();
  await page.waitForLoadState('domcontentloaded');
  const nodes = JSON.parse(kubectl(['get', 'nodes', '-o', 'json'])) as { items: unknown[] };
  await expect(page.getByRole('group', { name: 'Cluster overview' })).toContainText(
    `${String(nodes.items.length)} ready`,
    { timeout: 90_000 },
  );
});

test('the command palette searches every cluster and opens the hit where it lives', async ({
  page,
}) => {
  await openHome(page);
  await ensureSecond(page);
  const clusters = await opened(page);
  const original = clusters.find((one) => one.active);
  if (original === undefined) {
    throw new Error('no cluster was active before the fleet search');
  }
  const primary = idOf(clusters, CONTEXT);
  const second = idOf(clusters, SECOND_CONTEXT);
  const pod = kubectlSecond([
    '-n',
    'kube-system',
    'get',
    'pods',
    '-l',
    'k8s-app=kube-dns',
    '-o',
    'jsonpath={.items[0].metadata.name}',
  ]).trim();
  expect(pod).not.toBe('');
  try {
    await activate(page, primary);
    await openPalette(page);
    const searched = page.waitForResponse((response) => {
      const url = new URL(response.url());
      return url.pathname === '/api/search/fleet' && url.searchParams.get('q') === pod;
    });
    await page
      .getByRole('textbox', { name: 'Search resources, views and recent objects' })
      .fill(pod);
    const response = await searched;
    expect(response.ok()).toBe(true);
    const body = (await response.json()) as {
      hits: { cluster?: string; namespace: string; name: string }[];
    };
    expect(body.hits).toContainEqual(
      expect.objectContaining({ cluster: second, namespace: 'kube-system', name: pod }),
    );
    const hit = page.getByRole('dialog', { name: 'Command palette' }).getByRole('button', {
      name: `kube-system/${pod} pod · ${SECOND_CONTEXT}`,
      exact: true,
    });
    await expect(hit).toBeVisible({ timeout: 60_000 });
    await hit.click();
    await expect
      .poll(async () => (await opened(page)).find((one) => one.active)?.id, { timeout: 60_000 })
      .toBe(second);
    await expect(page).toHaveTitle(new RegExp(`^${pod} issues ${SECOND_CONTEXT} `), {
      timeout: 60_000,
    });
    await expect(page.getByRole('tablist', { name: 'right panels' })).toBeVisible();
  } finally {
    expect(await changeCluster(page, 'active', { cluster: original.id })).toBe(200);
    await expect
      .poll(async () => (await opened(page)).find((one) => one.active)?.id, { timeout: 60_000 })
      .toBe(original.id);
  }
});

test('a cluster tab name and group are stored, rendered, and reversible', async ({ page }) => {
  await openHome(page);
  await ensureSecond(page);
  const clusters = await opened(page);
  const id = idOf(clusters, SECOND_CONTEXT);
  const original = clusters.find((one) => one.context === SECOND_CONTEXT);
  if (original === undefined) {
    throw new Error(`no open cluster for ${SECOND_CONTEXT}`);
  }
  let originalLabel = '';
  if (original.label !== undefined) {
    originalLabel = original.label;
  }
  let originalGrouping = '';
  if (original.grouping !== undefined) {
    originalGrouping = original.grouping;
  }
  try {
    await openSettings(page, SECOND_CONTEXT);
    const settings = page.getByRole('group', { name: `Settings for ${SECOND_CONTEXT}` });
    await settings.getByLabel('Name', { exact: true }).fill('secondary');
    await settings.getByLabel('Group', { exact: true }).fill('test fleet');
    await settings.getByRole('button', { name: 'Save', exact: true }).click();
    await expect
      .poll(async () => (await opened(page)).find((one) => one.context === SECOND_CONTEXT)?.label)
      .toBe('secondary');
    await expect(page.getByRole('button', { name: 'secondary', exact: true })).toBeVisible();
    await expect(page.getByText('test fleet', { exact: true })).toBeVisible();
  } finally {
    expect(
      await changeCluster(page, 'name', {
        cluster: id,
        label: originalLabel,
        grouping: originalGrouping,
      }),
    ).toBe(200);
  }
  await expect
    .poll(async () => {
      const label = (await opened(page)).find((one) => one.context === SECOND_CONTEXT)?.label;
      if (label === undefined) {
        return '';
      }
      return label;
    })
    .toBe(originalLabel);
  await page.reload();
  await page.waitForLoadState('domcontentloaded');
  let rendered = SECOND_CONTEXT;
  if (originalLabel !== '') {
    rendered = originalLabel;
  }
  await expect(page.getByRole('button', { name: rendered, exact: true })).toBeVisible();
  if (originalGrouping !== '') {
    await expect(page.getByText(originalGrouping, { exact: true })).toBeVisible();
  }
});

test('a cluster tab colour is persisted and can be restored', async ({ page }) => {
  await openHome(page);
  await ensureSecond(page);
  const before = (await opened(page)).find((one) => one.context === SECOND_CONTEXT)?.color;
  expect(before).not.toBeUndefined();
  if (before === undefined) {
    throw new Error('the second cluster has no colour');
  }
  let next = 1;
  if (before === next) {
    next = 2;
  }
  const id = idOf(await opened(page), SECOND_CONTEXT);
  try {
    await openSettings(page, SECOND_CONTEXT);
    const settings = page.getByRole('group', { name: `Settings for ${SECOND_CONTEXT}` });
    await settings.getByRole('button', { name: `Colour ${String(next)}` }).click();
    await expect
      .poll(async () => (await opened(page)).find((one) => one.context === SECOND_CONTEXT)?.color)
      .toBe(next);
  } finally {
    expect(await changeCluster(page, 'color', { cluster: id, color: String(before) })).toBe(200);
  }
  await expect
    .poll(async () => (await opened(page)).find((one) => one.context === SECOND_CONTEXT)?.color)
    .toBe(before);
});

test('reopening a cluster next time is a persisted, reversible choice', async ({ page }) => {
  await openHome(page);
  await ensureSecond(page);
  const before = (await opened(page)).find((one) => one.context === SECOND_CONTEXT)?.reopen;
  expect(before).not.toBeUndefined();
  if (before === undefined) {
    throw new Error('the second cluster has no reopen state');
  }
  const id = idOf(await opened(page), SECOND_CONTEXT);
  try {
    await openSettings(page, SECOND_CONTEXT);
    const choice = page.getByRole('checkbox', { name: 'Open this cluster again next time' });
    await choice.click();
    await expect
      .poll(async () => (await opened(page)).find((one) => one.context === SECOND_CONTEXT)?.reopen)
      .toBe(!before);
  } finally {
    expect(await changeCluster(page, 'reopen', { cluster: id, reopen: String(before) })).toBe(200);
  }
  await expect
    .poll(async () => (await opened(page)).find((one) => one.context === SECOND_CONTEXT)?.reopen)
    .toBe(before);
});

test('closing the second cluster leaves the first one serving', async ({ page }) => {
  await openHome(page);
  await ensureSecond(page);
  await page.evaluate(
    async (id) => {
      await fetch(`/api/clusters?cluster=${encodeURIComponent(id)}`, { method: 'DELETE' });
    },
    idOf(await opened(page), SECOND_CONTEXT),
  );

  await expect.poll(async () => (await opened(page)).length, { timeout: 60_000 }).toBe(1);
  expect((await opened(page))[0].context).toBe(CONTEXT);
  await expect(page.getByRole('status', { name: 'The cluster feed is connected' })).toBeVisible({
    timeout: 60_000,
  });
});

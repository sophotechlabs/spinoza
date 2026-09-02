import { expect, test } from '../harness/test';
import { openHome } from '../harness/app';
import { CONTEXT, NOWHERE_CONTEXT, NOWHERE_KUBECONFIG } from '../harness/paths';
import type { Page } from '@playwright/test';

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
  const result = await page.evaluate(async (path) => {
    const response = await fetch(`/api/kubeconfigs?path=${encodeURIComponent(path)}`, {
      method: 'POST',
    });
    return { status: response.status, body: await response.text() };
  }, NOWHERE_KUBECONFIG);
  expect(result.status, result.body).toBe(200);
}

async function removeSecond(page: Page): Promise<void> {
  const result = await page.evaluate(async (path) => {
    const response = await fetch(`/api/kubeconfigs?path=${encodeURIComponent(path)}`, {
      method: 'DELETE',
    });
    return { status: response.status, body: await response.text() };
  }, NOWHERE_KUBECONFIG);
  expect(result.status, result.body).toBe(200);
}

async function secondManaged(page: Page): Promise<boolean> {
  return page.evaluate(async (path) => {
    const response = await fetch('/api/contexts');
    if (!response.ok) {
      throw new Error(`reading contexts returned ${String(response.status)}`);
    }
    const body = (await response.json()) as { kubeconfigs?: { path?: string }[] };
    if (!Array.isArray(body.kubeconfigs)) {
      return false;
    }
    return body.kubeconfigs.some((entry) => entry.path === path);
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

async function openManager(page: Page): Promise<void> {
  const loaded = page.waitForResponse((response) => {
    const url = new URL(response.url());
    return url.pathname === '/api/contexts' && response.request().method() === 'GET';
  });
  await openHome(page);
  await loaded;
  await page.getByLabel('Kubernetes context').click();
  await page.getByRole('button', { name: 'Manage kubeconfigs', exact: true }).click();
  await expect(page.getByRole('dialog', { name: 'Kubeconfigs' })).toBeVisible();
}

test.afterAll(async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();
  await openHome(page);
  if (await secondManaged(page)) {
    await removeSecond(page);
  }
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
  const picker = page.getByLabel('Kubernetes context');
  await expect(picker).toContainText(CONTEXT, { timeout: 30_000 });
  await picker.click();
  await expect(page.getByRole('button', { name: 'Manage kubeconfigs', exact: true })).toBeVisible();
});

test('a second kubeconfig puts its context in the picker', async ({ page }) => {
  await openHome(page);
  const before = await secondManaged(page);
  if (!before) {
    await addSecond(page);
  }
  try {
    await page.reload();
    await page.waitForLoadState('domcontentloaded');
    await page.locator('header').getByRole('group').first().click();
    await expect(
      page.locator('header').getByRole('button', { name: NOWHERE_CONTEXT, exact: true }),
    ).toBeVisible({ timeout: 30_000 });
  } finally {
    if (!before) {
      await removeSecond(page);
    }
  }
});

test('a cluster that never answers is refused, not added as a broken one', async ({ page }) => {
  await openHome(page);
  const refused = await openSecond(page);
  expect(refused.status).toBe(503);
  expect(refused.body).not.toBe('');

  await expect.poll(async () => (await opened(page)).length, { timeout: 30_000 }).toBe(1);
  expect((await opened(page))[0].context).toBe(CONTEXT);
});

test('a failed cluster switch is retained in failure notifications', async ({ page }) => {
  await openHome(page);
  const before = await secondManaged(page);
  if (!before) {
    await addSecond(page);
  }
  await page.reload();
  try {
    await page.getByLabel('Kubernetes context').click();
    await page.getByRole('button', { name: NOWHERE_CONTEXT, exact: true }).click();
    await expect(page.getByRole('status', { name: 'Latest notifications' })).toContainText(
      `Opening ${NOWHERE_CONTEXT}`,
      { timeout: 60_000 },
    );
    await page.getByLabel('Notifications', { exact: true }).click();
    const failures = page.getByRole('button', { name: 'Failures', exact: true });
    await failures.click();
    await expect(failures).toHaveAttribute('aria-pressed', 'true');
    await expect(page.getByText(new RegExp(`Opening ${NOWHERE_CONTEXT}`)).last()).toBeVisible();
    const clusters = await opened(page);
    expect(clusters).toHaveLength(1);
    expect(clusters[0].context).toBe(CONTEXT);
  } finally {
    if (!before) {
      await removeSecond(page);
    }
  }
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

test('the kubeconfig manager names each source and how many contexts it contributed', async ({
  page,
}) => {
  await openHome(page);
  const before = await secondManaged(page);
  if (!before) {
    await addSecond(page);
  }
  try {
    await openManager(page);
    const dialog = page.getByRole('dialog', { name: 'Kubeconfigs' });
    await expect(dialog.getByRole('listitem')).toHaveCount(2);
    const source = dialog.getByRole('listitem').filter({ hasText: 'kubeconfig-nowhere' });
    await expect(source).toContainText('1 context');
    await expect(
      source.getByRole('button', { name: /Remove .*kubeconfig-nowhere$/ }),
    ).toBeVisible();
  } finally {
    if (!before) {
      await removeSecond(page);
    }
  }
});

test('the kubeconfig manager diagnoses an empty path before making a request', async ({ page }) => {
  await openManager(page);
  const dialog = page.getByRole('dialog', { name: 'Kubeconfigs' });
  let requests = 0;
  page.on('request', (request) => {
    if (request.url().includes('/api/kubeconfigs?') && request.method() === 'POST') {
      requests += 1;
    }
  });
  await dialog.getByRole('button', { name: 'Add', exact: true }).click();
  await expect(dialog.getByRole('status')).toHaveText('type the path of a kubeconfig file');
  expect(requests).toBe(0);
});

test('the browser kubeconfig manager explains why it asks for a path instead of a file dialog', async ({
  page,
}) => {
  await openManager(page);
  const support = await page.evaluate(async () => {
    const response = await fetch('/api/kubeconfigs/picker');
    if (!response.ok) {
      throw new Error(`file-picker support returned ${response.status}`);
    }
    return response.json() as Promise<{ available: boolean; reason?: string }>;
  });
  expect(support.available).toBe(false);
  expect(support.reason).toContain('type the path instead');
  const dialog = page.getByRole('dialog', { name: 'Kubeconfigs' });
  await expect(dialog.getByRole('button', { name: 'Browse', exact: true })).toHaveCount(0);
  await expect(dialog.getByLabel('Add a kubeconfig')).toBeVisible();
});

test('a missing kubeconfig is refused and does not enter the managed list', async ({ page }) => {
  await openManager(page);
  const dialog = page.getByRole('dialog', { name: 'Kubeconfigs' });
  const missing = '/definitely/not/a/kubeconfig-e2e.yaml';
  await dialog.getByLabel('Add a kubeconfig').fill(missing);
  await dialog.getByRole('button', { name: 'Add', exact: true }).click();
  await expect(dialog.getByRole('status')).toContainText(/adding|failed|no such|not found/i, {
    timeout: 30_000,
  });
  const contexts = await page.evaluate(async () => {
    const response = await fetch('/api/contexts');
    return response.text();
  });
  expect(contexts).not.toContain(missing);
});

test('removing a kubeconfig removes its context from the picker and adding it back restores it', async ({
  page,
}) => {
  await openHome(page);
  const before = await secondManaged(page);
  if (!before) {
    await addSecond(page);
  }
  await page.reload();
  try {
    await openManager(page);
    const dialog = page.getByRole('dialog', { name: 'Kubeconfigs' });
    await dialog.getByRole('button', { name: /Remove .*kubeconfig-nowhere$/ }).click();
    await dialog.getByRole('button', { name: 'Close', exact: true }).click();
    await page.getByLabel('Kubernetes context').click();
    await expect(page.getByRole('button', { name: NOWHERE_CONTEXT, exact: true })).toHaveCount(0);
    await addSecond(page);
    await page.reload();
    await page.getByLabel('Kubernetes context').click();
    await expect(page.getByRole('button', { name: NOWHERE_CONTEXT, exact: true })).toBeVisible({
      timeout: 30_000,
    });
  } finally {
    const after = await secondManaged(page);
    if (before && !after) {
      await addSecond(page);
    }
    if (!before && after) {
      await removeSecond(page);
    }
  }
});

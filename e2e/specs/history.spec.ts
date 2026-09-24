import { expect, test } from '../harness/test';
import { openGrouped, openView, selectRow } from '../harness/app';
import { kubectl } from '../harness/cluster';
import { NAMESPACE } from '../harness/paths';
import type { Page } from '@playwright/test';

function replicas(): string {
  return kubectl([
    '-n',
    NAMESPACE,
    'get',
    'deployment/chatty',
    '-o',
    'jsonpath={.spec.replicas}',
  ]).trim();
}

async function recordScale(page: Page): Promise<void> {
  const original = replicas();
  let next = '3';
  if (original === next) {
    next = '2';
  }
  try {
    await openGrouped(page, 'apps', 'deployments', 'Deployment');
    await selectRow(page, 'chatty');
    await page.getByRole('tab', { name: 'Overview', exact: true }).click();
    const input = page.getByRole('spinbutton', { name: 'replicas' });
    await expect(input).toBeVisible({ timeout: 30_000 });
    await input.fill(next);
    await page.getByRole('button', { name: 'Scale', exact: true }).click();
    await expect.poll(replicas, { timeout: 60_000 }).toBe(next);
    await openView(page, 'history');
    await expect(page.locator('main')).toContainText('chatty', { timeout: 60_000 });
  } finally {
    kubectl(['-n', NAMESPACE, 'scale', 'deployment/chatty', `--replicas=${original}`]);
    await expect.poll(replicas, { timeout: 60_000 }).toBe(original);
  }
}

test('the view says what it is for', async ({ page }) => {
  await openView(page, 'history');
  const showing = page.getByRole('combobox', { name: 'What to show' });
  await expect(showing).toBeVisible({ timeout: 60_000 });
  await expect(showing).toContainText('What I did');
  await expect(showing).toContainText('What changed');
  await expect(page.getByRole('combobox', { name: 'What to record' })).toBeVisible();
});

test('a write made in the browser turns up in the history', async ({ page }) => {
  await recordScale(page);
});

test('the history names the object and what was done to it', async ({ page }) => {
  await recordScale(page);
  const main = page.locator('main');
  await expect(main).toContainText('chatty', { timeout: 60_000 });
  await expect(main).toContainText('scale');
  await expect(main).toContainText('deployments');
  for (const column of ['When', 'Did', 'To', 'Namespace', 'Outcome']) {
    await expect(main).toContainText(column);
  }
});

test('history source filters issue distinct backend requests', async ({ page }) => {
  await openView(page, 'history');
  const requests: string[] = [];
  page.on('request', (request) => {
    if (request.url().includes('/api/history?')) {
      requests.push(request.url());
    }
  });
  const source = page.getByRole('combobox', { name: 'What to show' });
  await expect(source.getByRole('option')).toHaveText(['Everything', 'What changed', 'What I did']);
  await source.selectOption('change');
  await expect.poll(() => requests.some((url) => url.includes('source=change'))).toBe(true);
  await source.selectOption('action');
  await expect.poll(() => requests.some((url) => url.includes('source=action'))).toBe(true);
  await source.selectOption('all');
  await expect(source).toHaveValue('all');
});

test('recording scope changes on the server and returns to its starting value', async ({
  page,
}) => {
  await openView(page, 'history');
  const recording = page.getByRole('combobox', { name: 'What to record' });
  const original = await recording.inputValue();
  let changed = 'workloads';
  if (original === changed) {
    changed = 'wide';
  }
  await expect(recording.getByRole('option')).toHaveText([
    'Recording nothing',
    'Recording workloads',
    'Recording workloads, network and GitOps',
  ]);
  try {
    const enabled = page.waitForResponse((response) => {
      const url = new URL(response.url());
      return url.pathname === '/api/clusters/timeline' && url.searchParams.get('kinds') === changed;
    });
    await recording.selectOption(changed);
    await enabled;
    await expect(recording).toHaveValue(changed);
  } finally {
    if ((await recording.inputValue()) !== original) {
      const restored = page.waitForResponse((response) => {
        const url = new URL(response.url());
        return (
          url.pathname === '/api/clusters/timeline' && url.searchParams.get('kinds') === original
        );
      });
      await recording.selectOption(original);
      await restored;
    }
  }
  await expect(recording).toHaveValue(original);
});

test('a history target reopens the object that was changed', async ({ page }) => {
  await recordScale(page);
  const target = page
    .locator('main tbody')
    .getByRole('button')
    .filter({ hasText: 'chatty' })
    .first();
  await expect(target).toBeVisible({ timeout: 60_000 });
  await target.click();
  await expect(page.getByRole('tablist', { name: 'right panels' })).toBeVisible({
    timeout: 60_000,
  });
  await expect(page).toHaveTitle(/^chatty history /);
});

test('history reports the live memory held while it watches', async ({ page }) => {
  await openView(page, 'history');
  await expect(page.getByText(/\d+ MB held/)).toBeVisible({ timeout: 60_000 });
});

test('clearing the history empties it and says so', async ({ page }) => {
  await recordScale(page);
  const clear = page.getByRole('button', { name: 'Clear', exact: true });
  await expect(clear).toBeEnabled();
  await clear.click();
  await expect(page.locator('main')).toContainText('There is nothing here yet.', {
    timeout: 30_000,
  });
  await expect(clear).toBeDisabled();
  const exported = await page.request.get('/api/history/export?format=json&source=action');
  expect(exported.status()).toBe(200);
  expect(await exported.json()).toEqual([]);
});

test('history exports preserve browser actions and paginate without repeating the cursor', async ({
  page,
}) => {
  await recordScale(page);
  await recordScale(page);
  const response = await page.request.get('/api/history/export?format=json&source=action&limit=2');
  expect(response.status()).toBe(200);
  expect(response.headers()['content-disposition']).toBe(
    'attachment; filename="spinoza-history.json"',
  );
  const entries = (await response.json()) as {
    id: number;
    source: string;
    verb: string;
    name: string;
    outcome: string;
    namespace: string;
  }[];
  expect(entries).toHaveLength(2);
  for (const entry of entries) {
    expect(entry).toMatchObject({
      source: 'action',
      verb: 'scale',
      name: 'chatty',
      namespace: NAMESPACE,
      outcome: 'done',
    });
  }
  expect(entries[0].id).toBeGreaterThan(entries[1].id);
  const older = await page.request.get(
    `/api/history/export?format=json&source=action&limit=1&afterAction=${String(entries[0].id)}`,
  );
  expect(older.status()).toBe(200);
  expect(await older.json()).toEqual([entries[1]]);
  const changes = await page.request.get('/api/history/export?format=json&source=change&limit=2');
  expect(changes.status()).toBe(200);
  for (const entry of (await changes.json()) as { source: string }[]) {
    expect(entry.source).toBe('change');
  }
});

test('a CSV history download names the browser action and preserves its identity', async ({
  page,
}) => {
  await recordScale(page);
  const json = await page.request.get('/api/history/export?format=json&source=action&limit=1');
  expect(json.status()).toBe(200);
  const [entry] = (await json.json()) as { id: number; cluster: string; at: string }[];
  const csv = await page.request.get('/api/history/export?format=csv&source=action&limit=1');
  expect(csv.status()).toBe(200);
  expect(csv.headers()['content-type']).toBe('text/csv; charset=utf-8');
  expect(csv.headers()['content-disposition']).toBe('attachment; filename="spinoza-history.csv"');
  const lines = (await csv.text()).trimEnd().split('\n');
  expect(lines).toHaveLength(2);
  expect(lines[0]).toBe(
    'id,source,cluster,at,verb,actor,group,version,resource,kind,namespace,name,detail,was,outcome,message',
  );
  expect(lines[1]).toContain(`${String(entry.id)},action,${entry.cluster},${entry.at},scale,`);
  expect(lines[1]).toContain(`,apps,v1,deployments,,${NAMESPACE},chatty,`);
  expect(lines[1]).toContain(',done,');
});

test('history export refuses an invalid source before starting a download and accepts a corrected request', async ({
  page,
}) => {
  await openView(page, 'history');
  const response = await page.request.get('/api/history/export?format=json&source=unknown');
  expect(response.status()).toBe(400);
  expect(response.headers()['content-disposition']).toBeUndefined();
  expect(await response.json()).toEqual({
    message: 'source must be all, action or change',
    request: expect.stringMatching(/\S+/),
  });
  const recovered = await page.request.get('/api/history/export?format=json&source=action&limit=1');
  expect(recovered.status()).toBe(200);
  expect(Array.isArray(await recovered.json())).toBe(true);
});

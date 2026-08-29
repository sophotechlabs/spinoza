import { expect, test, holdSide, sideAuthed } from '../harness/test';
import type { Browser, Page } from '@playwright/test';

test.describe.configure({ mode: 'serial' });

let release: () => Promise<void>;

test.beforeAll(async ({ browser }: { browser: Browser }) => {
  release = await holdSide(browser, 'readonly');
});

test.afterAll(async () => {
  await release();
});

async function openReadonly(browser: Browser, hash: string): Promise<[Page, () => Promise<void>]> {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(sideAuthed('readonly', hash));
  await page.waitForLoadState('domcontentloaded');
  return [page, async () => context.close()];
}

test('a cluster a limited user can reach still connects', async ({ browser }) => {
  const [page, close] = await openReadonly(browser, '');
  await expect(page.getByRole('status', { name: 'The cluster feed is connected' })).toBeVisible({
    timeout: 60_000,
  });
  await close();
});

test('what the user may not list is named, not silently dropped', async ({ browser }) => {
  const [page, close] = await openReadonly(browser, '');
  const partial = page.getByRole('status').filter({ hasText: 'Partial data' });
  await expect(partial).toBeVisible({ timeout: 60_000 });
  await expect(partial).toContainText('could not be listed');
  await expect(partial).toContainText('forbidden');
  await expect(partial.getByRole('button', { name: 'Show more' })).toBeVisible();
  await close();
});

test('the overview still reports what the user is allowed to read', async ({ browser }) => {
  const [page, close] = await openReadonly(browser, '');
  const overview = page.getByRole('group', { name: 'Cluster overview' });
  await expect(overview).toContainText(/Kubernetes\s*v\d+/, { timeout: 60_000 });
  await expect(overview).toContainText(/\d+ ready/);
  await close();
});

test('a metric with no permission to read it is not invented', async ({ browser }) => {
  const [page, close] = await openReadonly(browser, '');
  await expect(page.getByRole('group', { name: 'Cluster overview' })).toContainText(
    'Live usage needs metrics-server; only the allocatable totals are shown.',
    { timeout: 60_000 },
  );
  await close();
});

test('releases a user cannot read are reported as none, not as an empty table', async ({
  browser,
}) => {
  const [page, close] = await openReadonly(browser, '#view=helm');
  await expect(page.locator('main')).toContainText('No Helm releases in this cluster.', {
    timeout: 60_000,
  });
  await close();
});

test('a limited user is told which namespaces refused the read', async ({ browser }) => {
  const [page, close] = await openReadonly(browser, '#view=helm');
  await expect(page.getByRole('status').filter({ hasText: 'Partial data' })).toContainText(
    'secrets could not be listed cluster-wide',
    { timeout: 60_000 },
  );
  await close();
});

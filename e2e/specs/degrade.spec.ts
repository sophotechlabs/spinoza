import { expect, test, holdSide, sideAuthed } from '../harness/test';
import type { Held } from '../harness/keepalive';
import { CONTEXT } from '../harness/paths';
import type { Browser, Page } from '@playwright/test';

let release: Held;
let releaseToolless: Held;

test.beforeAll(() => {
  release = holdSide('nowhere');
  releaseToolless = holdSide('toolless');
});

test.afterAll(async () => {
  await release.close();
  await releaseToolless.close();
});

async function openNowhere(browser: Browser, hash: string): Promise<[Page, () => Promise<void>]> {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(sideAuthed('nowhere', hash));
  await page.waitForLoadState('domcontentloaded');
  return [page, async () => context.close()];
}

test('a cluster that never answered is named as such, not left blank', async ({ browser }) => {
  const [page, close] = await openNowhere(browser, '');
  await expect(page.getByRole('banner')).toContainText('no cluster', { timeout: 60_000 });
  await close();
});

test('the feed says it is disconnected rather than pretending', async ({ browser }) => {
  const [page, close] = await openNowhere(browser, '');
  await expect(page.getByRole('status', { name: 'The cluster feed is disconnected' })).toBeVisible({
    timeout: 60_000,
  });
  await close();
});

test('the app says it is retrying, and how many times it has tried', async ({ browser }) => {
  const [page, close] = await openNowhere(browser, '');
  const dropped = page.getByRole('status', { name: 'The cluster feed dropped' });
  await expect(dropped).toContainText('The live connection dropped', { timeout: 60_000 });
  await expect(dropped).toContainText(/Reconnecting, attempt \d+/);
  await expect(dropped.getByRole('button', { name: 'Reconnect now' })).toBeVisible();
  await close();
});

test('discovery that failed says so and offers to try again', async ({ browser }) => {
  const [page, close] = await openNowhere(browser, '');
  const failure = page.getByRole('alert').filter({ hasText: 'Discovery failed' });
  await expect(failure).toBeVisible({ timeout: 60_000 });
  await expect(failure).toContainText('discovery request failed');
  await expect(failure.getByRole('button', { name: 'Retry' })).toBeVisible();
  await close();
});

test('retrying failed discovery sends a fresh discovery request', async ({ browser }) => {
  const [page, close] = await openNowhere(browser, '');
  const failure = page.getByRole('alert').filter({ hasText: 'Discovery failed' });
  await expect(failure).toBeVisible({ timeout: 60_000 });
  const retried = page.waitForRequest(
    (request) => request.url().includes('/api/resources') && request.method() === 'POST',
    { timeout: 30_000 },
  );
  await failure.getByRole('button', { name: 'Retry', exact: true }).click();
  await retried;
  await expect(failure).toContainText('discovery request failed', { timeout: 60_000 });
  await close();
});

test('a view that cannot load says why, in the words the backend used', async ({ browser }) => {
  const [page, close] = await openNowhere(browser, '');
  await expect(
    page.getByRole('alert').filter({ hasText: 'The cluster overview could not be loaded' }),
  ).toContainText('spinoza has no cluster; pick a context that answers', { timeout: 60_000 });
  await close();
});

test('the views that need a cluster are the ones that go quiet', async ({ browser }) => {
  const [page, close] = await openNowhere(browser, '');
  const bottom = page.getByRole('tablist', { name: 'bottom panels' });
  await expect(bottom.getByRole('tab', { name: 'Compare', exact: true })).toBeDisabled({
    timeout: 60_000,
  });
  await expect(bottom.getByRole('tab', { name: 'Forwards', exact: true })).toBeEnabled();
  await close();
});

test('a helm spinoza cannot run is not offered as a button that would fail', async ({
  browser,
}) => {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(sideAuthed('toolless', `#context=${CONTEXT}&view=helm`));
  await page.waitForLoadState('domcontentloaded');
  await expect(page.getByRole('button', { name: 'Install chart' })).toBeDisabled({
    timeout: 60_000,
  });
  await context.close();
});

for (const endpoint of ['/api/overview', '/api/checks', '/api/resources/counts']) {
  test(`${endpoint} does not describe a total cluster failure as a successful empty result`, async ({
    browser,
  }) => {
    const [page, close] = await openNowhere(browser, '');
    const status = await page.evaluate(async (path) => {
      const response = await fetch(path);
      return response.status;
    }, endpoint);
    expect(status).toBeGreaterThanOrEqual(400);
    await close();
  });
}

test('a side without node-shell support leaves the action disabled and explains the flag', async ({
  browser,
}) => {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(
    sideAuthed('toolless', `#context=${CONTEXT}&version=v1&resource=nodes&kind=Node`),
  );
  await page.waitForLoadState('domcontentloaded');
  const row = page.locator('main tbody tr').first();
  await expect(row).toBeVisible({ timeout: 60_000 });
  await row.getByRole('button').first().click();
  await page.getByRole('tab', { name: 'Overview', exact: true }).click();
  const shell = page.getByRole('button', { name: 'Node shell', exact: true });
  await expect(shell).toBeDisabled({ timeout: 30_000 });
  await expect(shell.locator('..')).toHaveAttribute('title', /--node-shell/);
  await context.close();
});

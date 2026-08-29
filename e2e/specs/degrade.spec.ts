import { expect, test, holdSide, sideAuthed } from '../harness/test';
import type { Browser, Page } from '@playwright/test';

test.describe.configure({ mode: 'serial' });

let release: () => Promise<void>;

test.beforeAll(async ({ browser }: { browser: Browser }) => {
  release = await holdSide(browser, 'nowhere');
});

test.afterAll(async () => {
  await release();
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

test('a view that cannot load says which request failed', async ({ browser }) => {
  const [page, close] = await openNowhere(browser, '');
  await expect(
    page.getByRole('alert').filter({ hasText: 'The cluster overview could not be loaded' }),
  ).toContainText('overview request failed with status 503', { timeout: 60_000 });
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

test('a helm spinoza cannot run is not offered as a button that would fail', async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();
  const toolless = await holdSide(browser, 'toolless');
  await page.goto(sideAuthed('toolless', '#view=helm'));
  await page.waitForLoadState('domcontentloaded');
  await expect(page.getByRole('button', { name: 'Install chart' })).toBeDisabled({
    timeout: 60_000,
  });
  await context.close();
  await toolless();
});

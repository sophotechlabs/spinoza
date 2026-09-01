import { expect, test, holdSide, sideAuthed } from '../harness/test';
import type { Held } from '../harness/keepalive';
import { CONTEXT } from '../harness/paths';
import type { Browser, Page } from '@playwright/test';

let release: Held;

test.beforeAll(() => {
  release = holdSide('traffic');
});

test.afterAll(async () => {
  await release.close();
});

async function openTraffic(browser: Browser): Promise<[Page, () => Promise<void>]> {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(sideAuthed('traffic', `#context=${CONTEXT}&view=traffic`));
  await page.waitForLoadState('domcontentloaded');
  return [page, async () => context.close()];
}

test('a mesh that is exporting flows makes the view reachable', async ({ browser }) => {
  const [page, close] = await openTraffic(browser);
  await expect(page.getByRole('button', { name: 'Traffic', exact: true })).toBeEnabled({
    timeout: 60_000,
  });
  await close();
});

test('the graph draws the workloads the flows name', async ({ browser }) => {
  const [page, close] = await openTraffic(browser);
  const main = page.locator('main');
  await expect(main).toContainText('healthy', { timeout: 90_000 });
  await expect(main).toContainText('chatty');
  await close();
});

test('the graph draws edges, not just a legend that lists them', async ({ browser }) => {
  const [page, close] = await openTraffic(browser);
  await expect(page.locator('.react-flow__node').first()).toBeVisible({ timeout: 90_000 });
  expect(await page.locator('.react-flow__node').count()).toBeGreaterThan(1);
  expect(await page.locator('.react-flow__edge').count()).toBeGreaterThan(0);
  await close();
});

test('the source of the numbers is named', async ({ browser }) => {
  const [page, close] = await openTraffic(browser);
  await expect(page.locator('main')).toContainText('Cilium Hubble', { timeout: 90_000 });
  await close();
});

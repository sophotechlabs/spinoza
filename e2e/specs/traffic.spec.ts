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

test('the graph keeps every workload named by the flow data', async ({ browser }) => {
  const [page, close] = await openTraffic(browser);
  const main = page.locator('main');
  for (const workload of ['e2e/healthy', 'e2e/chatty', 'e2e/risky', 'kube-system/coredns']) {
    await expect(main.getByText(workload, { exact: true })).toBeVisible({ timeout: 90_000 });
  }
  await close();
});

test('the traffic legend distinguishes forwarded and dropped flows', async ({ browser }) => {
  const [page, close] = await openTraffic(browser);
  const main = page.locator('main');
  await expect(main).toContainText('Forwarded flows per second', { timeout: 90_000 });
  await expect(main).toContainText('Some flows dropped');
  await close();
});

test('traffic edges have drawable geometry rather than empty paths', async ({ browser }) => {
  const [page, close] = await openTraffic(browser);
  const paths = page.locator('.react-flow__edge-path');
  await expect(paths.first()).toBeVisible({ timeout: 90_000 });
  const lengths = await paths.evaluateAll((elements) =>
    elements.map((element) => (element as SVGGeometryElement).getTotalLength()),
  );
  expect(lengths.length).toBeGreaterThan(0);
  for (const length of lengths) {
    expect(length).toBeGreaterThan(0);
  }
  await close();
});

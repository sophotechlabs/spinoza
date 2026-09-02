import { expect, test } from '../harness/test';
import { openView } from '../harness/app';

test('the graph explains its own edges', async ({ page }) => {
  await openView(page, 'topology');
  const main = page.locator('main');
  for (const label of ['Owns', 'Routes to', 'Configures', 'Scales']) {
    await expect(main).toContainText(label, { timeout: 90_000 });
  }
});

test('the graph draws the edges the api sent, not just the legend', async ({ page }) => {
  await openView(page, 'topology');
  await expect(page.locator('.react-flow__node').first()).toBeVisible({ timeout: 90_000 });

  const sent = await page.evaluate(async () => {
    const answer = await fetch('/api/topology').then((r) => r.json());
    return (answer.edges ?? []).length as number;
  });
  expect(sent).toBeGreaterThan(0);

  await expect
    .poll(() => page.locator('.react-flow__edge').count(), { timeout: 60_000 })
    .toBeGreaterThan(0);
});

test('every drawn edge joins two drawn nodes', async ({ page }) => {
  await openView(page, 'topology');
  await expect(page.locator('.react-flow__node').first()).toBeVisible({ timeout: 90_000 });
  await expect
    .poll(() => page.locator('.react-flow__edge').count(), { timeout: 60_000 })
    .toBeGreaterThan(0);

  const dangling = await page.evaluate(() => {
    const ids = new Set(
      [...document.querySelectorAll('.react-flow__node')].map((n) => n.getAttribute('data-id')),
    );
    return [...document.querySelectorAll('.react-flow__edge')]
      .map((edge) => edge.getAttribute('data-id') ?? '')
      .filter((id) => {
        const [from, rest] = id.split('->');
        const to = (rest ?? '').split(':')[0];
        return !ids.has(from) || !ids.has(to);
      });
  });

  expect(dangling).toEqual([]);
});

test('the graph distinguishes ready from not ready', async ({ page }) => {
  await openView(page, 'topology');
  const main = page.locator('main');
  await expect(main).toContainText('Ready', { timeout: 90_000 });
  await expect(main).toContainText('Not ready or missing');
});

test('the graph renders rather than staying on its loading state', async ({ page }) => {
  await openView(page, 'topology');
  await expect(page.locator('main')).not.toContainText('Loading graph', { timeout: 90_000 });
  await expect(page.locator('main svg').first()).toBeVisible();
});

test('an edge is drawn as a path with real geometry, not a zero-length line', async ({ page }) => {
  await openView(page, 'topology');
  await expect
    .poll(() => page.locator('.react-flow__edge-path').count(), { timeout: 90_000 })
    .toBeGreaterThan(0);
  const lengths = await page
    .locator('.react-flow__edge-path')
    .evaluateAll((nodes) => nodes.map((node) => (node as SVGPathElement).getTotalLength()));
  expect(lengths.filter((one) => one === 0)).toHaveLength(0);
});

test('namespace scope is sent to the topology backend and removes other namespaces', async ({
  page,
}) => {
  await openView(page, 'topology');
  await expect(page.locator('.react-flow__node').first()).toBeVisible({ timeout: 90_000 });
  const scoped = page.waitForRequest((request) => {
    const url = new URL(request.url());
    return url.pathname === '/api/topology' && url.searchParams.get('namespace') === 'e2e';
  });
  await page.getByRole('combobox', { name: 'Namespace', exact: true }).selectOption('e2e');
  await scoped;
  await expect(page.locator('.react-flow__node').filter({ hasText: 'coredns' })).toHaveCount(0, {
    timeout: 90_000,
  });
  await expect(
    page
      .locator('.react-flow__node')
      .filter({ hasText: /healthy/ })
      .first(),
  ).toBeVisible();
});

test('a leaf in the topology opens the object it represents', async ({ page }) => {
  await openView(page, 'topology');
  const scoped = page.waitForRequest((request) => {
    const url = new URL(request.url());
    return url.pathname === '/api/topology' && url.searchParams.get('namespace') === 'e2e';
  });
  await page.getByRole('combobox', { name: 'Namespace', exact: true }).selectOption('e2e');
  await scoped;
  const service = page.locator('.react-flow__node').filter({ hasText: /^healthy$/ });
  await expect(service).toBeVisible({ timeout: 90_000 });
  await page.getByRole('button', { name: 'Fit View', exact: true }).click();
  await service.click();
  await expect(page).toHaveTitle(/^healthy topology /, { timeout: 60_000 });
  await expect(page.getByRole('tablist', { name: 'right panels' })).toBeVisible();
});

test('a folded workload expands through a scoped topology request', async ({ page }) => {
  await openView(page, 'topology');
  const scoped = page.waitForRequest((request) => {
    const url = new URL(request.url());
    return url.pathname === '/api/topology' && url.searchParams.get('namespace') === 'e2e';
  });
  await page.getByRole('combobox', { name: 'Namespace', exact: true }).selectOption('e2e');
  await scoped;
  const workload = page.locator('.react-flow__node').filter({ hasText: /^healthy ×\d+/ });
  await expect(workload).toBeVisible({ timeout: 90_000 });
  await page.getByRole('button', { name: 'Fit View', exact: true }).click();
  const expanded = page.waitForRequest((request) => {
    const url = new URL(request.url());
    return url.pathname === '/api/topology' && url.searchParams.has('expand');
  });
  await workload.click();
  const request = await expanded;
  expect(new URL(request.url()).searchParams.get('expand')).not.toBe('');
  await expect(
    page
      .locator('.react-flow__node')
      .filter({ hasText: /^healthy-/ })
      .first(),
  ).toBeVisible({
    timeout: 90_000,
  });
});

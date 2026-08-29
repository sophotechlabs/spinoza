import { expect, test } from '../harness/test';
import { openView } from '../harness/app';

test('the graph explains its own edges', async ({ page }) => {
  await openView(page, 'topology');
  const main = page.locator('main');
  await expect(main).toContainText('Owns', { timeout: 90_000 });
  await expect(main).toContainText('Routes to');
  await expect(main).toContainText('Configures');
});

test('the graph draws the edges the api sent, not just the legend', async ({ page }) => {
  await openView(page, 'topology');
  await expect(page.locator('.react-flow__node').first()).toBeVisible({ timeout: 90_000 });

  const sent = await page.evaluate(async () => {
    const answer = await fetch('/api/topology').then((r) => r.json());
    return (answer.edges ?? []).length as number;
  });
  test.skip(sent === 0, 'this cluster has no relationships to draw');

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

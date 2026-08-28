import { expect, test } from '../harness/test';
import { openView } from '../harness/app';

test('the graph draws and explains its own edges', async ({ page }) => {
  await openView(page, 'topology');
  const main = page.locator('main');
  await expect(main).toContainText('Owns', { timeout: 90_000 });
  await expect(main).toContainText('Routes to');
  await expect(main).toContainText('Configures');
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

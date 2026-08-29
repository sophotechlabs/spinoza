import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '../harness/test';
import { openHome, openResource, openView } from '../harness/app';

const TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'];

const VIEWS = ['issues', 'topology', 'helm', 'checks'];

test('the cluster overview has no detectable accessibility violations', async ({ page }) => {
  await openHome(page);
  await page.waitForTimeout(2500);
  const results = await new AxeBuilder({ page }).withTags(TAGS).analyze();
  expect(results.violations).toEqual([]);
});

for (const view of VIEWS) {
  test(`the ${view} view has no detectable accessibility violations`, async ({ page }) => {
    await openView(page, view);
    await page.waitForTimeout(2500);
    const results = await new AxeBuilder({ page }).withTags(TAGS).analyze();
    expect(results.violations).toEqual([]);
  });
}

test('a resource table has no detectable accessibility violations', async ({ page }) => {
  await openResource(page, 'pods', 'Pod');
  await page.waitForTimeout(2500);
  const results = await new AxeBuilder({ page }).withTags(TAGS).analyze();
  expect(results.violations).toEqual([]);
});

test('the inspect drawer has no detectable accessibility violations', async ({ page }) => {
  await openResource(page, 'pods', 'Pod');
  const row = page.locator('main tbody tr').filter({ hasText: 'noshell' }).first();
  await expect(row).toBeVisible({ timeout: 60_000 });
  await row.click();
  await page.waitForTimeout(2000);
  const results = await new AxeBuilder({ page }).withTags(TAGS).analyze();
  expect(results.violations).toEqual([]);
});

test('the command palette has no detectable accessibility violations', async ({ page }) => {
  await openHome(page);
  await page.keyboard.press('ControlOrMeta+k');
  await expect(page.getByPlaceholder('Search')).toBeVisible();
  const results = await new AxeBuilder({ page }).withTags(TAGS).analyze();
  expect(results.violations).toEqual([]);
});

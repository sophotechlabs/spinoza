import { expect, test } from '../harness/test';
import { openView } from '../harness/app';

test('an empty cluster says so rather than showing an empty table', async ({ page }) => {
  await openView(page, 'helm');
  await expect(page.locator('main')).toContainText('No Helm releases in this cluster.', {
    timeout: 60_000,
  });
});

test('installing a chart is offered even with nothing installed', async ({ page }) => {
  await openView(page, 'helm');
  await expect(page.getByRole('button', { name: 'Install chart' })).toBeVisible({
    timeout: 60_000,
  });
});

test('the release count is reported honestly', async ({ page }) => {
  await openView(page, 'helm');
  await expect(page.locator('main')).toContainText('0 of 0', { timeout: 60_000 });
});

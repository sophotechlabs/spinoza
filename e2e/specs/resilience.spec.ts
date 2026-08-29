import { expect, test } from '../harness/test';
import { openHome, openView } from '../harness/app';

test('the app reports the health of its own feed', async ({ page }) => {
  await openHome(page);
  await expect(page.getByRole('status', { name: 'The cluster feed is connected' })).toBeVisible({
    timeout: 30_000,
  });
});

test('reconnecting is always offered, not only after a failure', async ({ page }) => {
  await openHome(page);
  await expect(page.getByRole('button', { name: 'Reconnect' })).toBeVisible();
});

test('a reconnect leaves the feed connected', async ({ page }) => {
  await openHome(page);
  await page.getByRole('button', { name: 'Reconnect' }).click();
  await expect(page.getByRole('status', { name: 'The cluster feed is connected' })).toBeVisible({
    timeout: 60_000,
  });
});

test('an integration that is absent is named and disabled, not hidden', async ({ page }) => {
  await openHome(page);
  for (const absent of ['Traffic', 'Flux', 'Argo CD']) {
    const button = page.getByRole('button', { name: absent, exact: true });
    await expect(button).toBeVisible();
    await expect(button).toBeDisabled();
  }
});

test('a cluster with no releases says so rather than showing nothing', async ({ page }) => {
  await openView(page, 'helm');
  await expect(page.locator('main')).toContainText('No Helm releases in this cluster.', {
    timeout: 60_000,
  });
});

test('a metric with no source is not invented', async ({ page }) => {
  await openHome(page);
  await expect(page.getByRole('heading', { name: 'Allocatable capacity' })).toBeVisible({
    timeout: 60_000,
  });
});

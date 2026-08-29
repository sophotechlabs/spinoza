import { expect, test } from '../harness/test';
import { openView } from '../harness/app';
import { kubectl } from '../harness/cluster';

test('the application list names what argo reports about each app', async ({ page }) => {
  await openView(page, 'argo-apps');
  const main = page.locator('main');
  for (const column of ['Application', 'Namespace', 'Sync', 'Health', 'Destination', 'Revision']) {
    await expect(main).toContainText(column, { timeout: 120_000 });
  }
});

test('an application that argo has not synced is reported as drifted', async ({ page }) => {
  await openView(page, 'argo-apps');
  const app = page.getByRole('button', { name: /^guestbook / });
  await expect(app).toBeVisible({ timeout: 120_000 });
  const sync = kubectl([
    '-n',
    'argocd',
    'get',
    'application/guestbook',
    '-o',
    'jsonpath={.status.sync.status}',
  ]).trim();
  expect(sync).not.toBe('');
  await expect(app).toContainText(sync);
});

test('the destination the application points at is shown, not assumed', async ({ page }) => {
  await openView(page, 'argo-apps');
  const app = page.getByRole('button', { name: /^guestbook / });
  await expect(app).toContainText('https://kubernetes.default.svc', { timeout: 120_000 });
  await expect(app).toContainText('e2e-gitops');
});

test('the graph draws the application argo is tracking', async ({ page }) => {
  await openView(page, 'argo-graph');
  await expect
    .poll(() => page.locator('.react-flow__node').count(), { timeout: 120_000 })
    .toBeGreaterThan(0);
  await expect(page.locator('main')).toContainText('guestbook');
});

test('the argo graph explains its own edges', async ({ page }) => {
  await openView(page, 'argo-graph');
  await expect(page.locator('main')).toContainText('Manages', { timeout: 120_000 });
});

test('the per-kind list opens without a cluster sync to hang it from', async ({ page }) => {
  await openView(page, 'argo-list');
  await expect(page).toHaveTitle(/^argo-list /, { timeout: 120_000 });
  await expect(page.locator('main')).not.toBeEmpty();
});

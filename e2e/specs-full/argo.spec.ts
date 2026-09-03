import { expect, test } from '../harness/test';
import { openView } from '../harness/app';
import { kubectl } from '../harness/cluster';
import type { Locator, Page } from '@playwright/test';

async function openGuestbook(page: Page): Promise<void> {
  await openView(page, 'argo-apps');
  const app = page.getByRole('button', { name: /^guestbook / });
  await expect(app).toBeVisible({ timeout: 120_000 });
  await app.click();
  await expect(page.getByRole('tab', { name: 'Overview', exact: true })).toBeVisible({
    timeout: 60_000,
  });
}

function application(path: string): string {
  return kubectl([
    '-n',
    'argocd',
    'get',
    'application/guestbook',
    '-o',
    `jsonpath={${path}}`,
  ]).trim();
}

function annotationValue(page: Page, name: string): Locator {
  const overview = page.getByRole('tabpanel', { name: 'Overview' });
  const annotations = overview
    .getByRole('heading', { name: 'Annotations', exact: true })
    .locator('..');
  const term = annotations.getByText(name, { exact: true });
  return term.locator('..').getByRole('definition').locator('span').first();
}

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

test('an application drawer carries its source, destination, and inspection modes', async ({
  page,
}) => {
  await openGuestbook(page);
  await page.getByRole('tab', { name: 'Application', exact: true }).click();
  const applicationPanel = page.getByRole('tabpanel', { name: 'Application' });
  await expect(applicationPanel).toContainText('https://github.com/argoproj/argocd-example-apps', {
    timeout: 120_000,
  });
  await expect(applicationPanel).toContainText('guestbook');
  await expect(applicationPanel).toContainText('HEAD');
  await expect(applicationPanel).toContainText('https://kubernetes.default.svc');
  for (const tab of ['Resources', 'Activity', 'Topology']) {
    await expect(applicationPanel.getByRole('button', { name: tab, exact: true })).toBeVisible();
  }
});

test('the Argo sync dialog exposes its safety and apply choices without writing on cancel', async ({
  page,
}) => {
  const before = application('.metadata.generation');
  await openGuestbook(page);
  await page.getByRole('tab', { name: 'Overview', exact: true }).click();
  await page.getByRole('button', { name: 'Sync', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Sync guestbook' });
  for (const choice of [
    /^Prune/,
    /^Dry run/,
    /^Apply only/,
    /^Force/,
    /^Replace/,
    /^Server-side apply/,
  ]) {
    await expect(dialog.getByRole('checkbox', { name: choice })).toBeVisible();
  }
  await dialog.getByRole('checkbox', { name: /^Force/ }).check();
  await expect(dialog).toContainText('The PreSync and PostSync hooks still run.');
  await dialog.getByRole('checkbox', { name: /^Apply only/ }).check();
  await expect(dialog).not.toContainText('The PreSync and PostSync hooks still run.');
  await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(application('.metadata.generation')).toBe(before);
});

test('refreshing an Argo application stamps the live object through the backend', async ({
  page,
}) => {
  await openGuestbook(page);
  await page.getByRole('tab', { name: 'Overview', exact: true }).click();
  await page.getByRole('button', { name: 'Refresh', exact: true }).click();
  await expect(page.getByText('Refresh requested.', { exact: true })).toBeVisible({
    timeout: 60_000,
  });
  await expect(annotationValue(page, 'argocd.argoproj.io/refresh')).toHaveText('normal', {
    timeout: 60_000,
  });
});

test('hard refreshing an Argo application requests an uncached repository read', async ({
  page,
}) => {
  await openGuestbook(page);
  await page.getByRole('tab', { name: 'Overview', exact: true }).click();
  await page.getByRole('button', { name: 'Hard refresh', exact: true }).click();
  await expect(page.getByText('Hard refresh requested.', { exact: true })).toBeVisible({
    timeout: 60_000,
  });
  await expect(annotationValue(page, 'argocd.argoproj.io/refresh')).toHaveText('hard', {
    timeout: 60_000,
  });
});

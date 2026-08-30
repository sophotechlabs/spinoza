import { expect, test } from '../harness/test';
import { openGrouped, openResource, selectRow } from '../harness/app';
import { kubectl } from '../harness/cluster';
import { NAMESPACE } from '../harness/paths';
import type { Page } from '@playwright/test';

test.describe.configure({ mode: 'serial' });

function field(kind: string, name: string, path: string, namespaced = true): string {
  const args = ['get', `${kind}/${name}`, '-o', `jsonpath={${path}}`];
  if (namespaced) {
    return kubectl(['-n', NAMESPACE, ...args]).trim();
  }
  return kubectl(args).trim();
}

async function openWorkload(page: Page, name: string): Promise<void> {
  await openGrouped(page, 'apps', 'deployments', 'Deployment');
  await selectRow(page, name);
  await page.getByRole('tab', { name: 'Overview', exact: true }).click();
}

async function press(page: Page, name: string): Promise<void> {
  await page.getByRole('button', { name, exact: true }).click();
  const confirm = page.getByRole('button', { name: 'Confirm', exact: true });
  await confirm.first().click({ timeout: 5_000 }).catch(() => undefined);
}

async function openNode(page: Page, name: string): Promise<void> {
  await openResource(page, 'nodes', 'Node');
  await selectRow(page, name);
  await page.getByRole('tab', { name: 'Overview', exact: true }).click();
}

test('scaling in the browser moves the apiserver', async ({ page }) => {
  await openWorkload(page, 'healthy');
  const replicas = page.getByRole('spinbutton', { name: 'replicas' });
  await expect(replicas).toHaveValue('2', { timeout: 30_000 });
  await replicas.fill('3');
  await press(page, 'Scale');
  await expect
    .poll(() => field('deployment', 'healthy', '.spec.replicas'), { timeout: 60_000 })
    .toBe('3');
  await expect(page.locator('main tbody tr').filter({ hasText: 'healthy' }).first()).toContainText(
    '3',
    { timeout: 60_000 },
  );
});

test('scaling back down is the same round trip', async ({ page }) => {
  await openWorkload(page, 'healthy');
  const replicas = page.getByRole('spinbutton', { name: 'replicas' });
  await expect(replicas).toHaveValue('3', { timeout: 60_000 });
  await replicas.fill('2');
  await press(page, 'Scale');
  await expect
    .poll(() => field('deployment', 'healthy', '.spec.replicas'), { timeout: 60_000 })
    .toBe('2');
});

test('a rollout restart stamps the pod template the apiserver keeps', async ({ page }) => {
  const stamp = (): string =>
    field('deployment', 'chatty', '.spec.template.metadata.annotations');
  const before = stamp();
  await openWorkload(page, 'chatty');
  await press(page, 'Restart');
  await expect.poll(stamp, { timeout: 60_000 }).not.toBe(before);
  expect(stamp()).toContain('restartedAt');
});

test('cordoning a node takes it out of scheduling, and uncordoning puts it back', async ({
  page,
}) => {
  const node = kubectl([
    'get',
    'nodes',
    '-l',
    'spinoza.test/pool=drain',
    '-o',
    'jsonpath={.items[0].metadata.name}',
  ]).trim();
  expect(node).not.toBe('');
  await openNode(page, node);
  await press(page, 'Cordon');
  await expect
    .poll(() => field('node', node, '.spec.unschedulable', false), { timeout: 60_000 })
    .toBe('true');
  await expect(page.getByRole('button', { name: 'Uncordon', exact: true })).toBeVisible({
    timeout: 30_000,
  });
  await press(page, 'Uncordon');
  await expect
    .poll(() => field('node', node, '.spec.unschedulable', false), { timeout: 60_000 })
    .toBe('');
});

test('suspending a cronjob reaches the apiserver, and resuming undoes it', async ({ page }) => {
  await openGrouped(page, 'batch', 'cronjobs', 'CronJob');
  await selectRow(page, 'nightly');
  await page.getByRole('tab', { name: 'Overview', exact: true }).click();
  await press(page, 'Suspend');
  await expect
    .poll(() => field('cronjob', 'nightly', '.spec.suspend'), { timeout: 60_000 })
    .toBe('true');
  await press(page, 'Resume');
  await expect
    .poll(() => field('cronjob', 'nightly', '.spec.suspend'), { timeout: 60_000 })
    .toBe('false');
});

test('running a cronjob now creates the job it would have created on schedule', async ({
  page,
}) => {
  await openGrouped(page, 'batch', 'cronjobs', 'CronJob');
  await selectRow(page, 'nightly');
  await page.getByRole('tab', { name: 'Overview', exact: true }).click();
  await press(page, 'Run now');
  await expect
    .poll(
      () => kubectl(['-n', NAMESPACE, 'get', 'jobs', '-o', 'jsonpath={.items[*].metadata.name}']),
      { timeout: 60_000 },
    )
    .toContain('nightly');
});

test('a bulk restart names how many objects it is about to touch', async ({ page }) => {
  await openGrouped(page, 'apps', 'deployments', 'Deployment');
  await page.getByRole('checkbox', { name: 'Select healthy' }).check();
  await page.getByRole('checkbox', { name: 'Select chatty' }).check();
  const bar = page.locator('main').getByRole('status').filter({ hasText: 'selected' });
  await expect(bar).toContainText(/2\s*Deployment objects selected/, { timeout: 30_000 });
  for (const action of ['Restart', 'Delete', 'Clear selection']) {
    await expect(bar.getByRole('button', { name: action, exact: true })).toBeVisible();
  }
  await page.getByRole('button', { name: 'Clear selection', exact: true }).click();
  await expect(page.getByRole('checkbox', { name: 'Select healthy' })).not.toBeChecked();
});

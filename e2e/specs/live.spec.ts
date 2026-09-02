import { expect, test } from '../harness/test';
import { openResource, selectRow } from '../harness/app';
import { authed } from '../harness/test';
import { kubectl } from '../harness/cluster';
import { CONTEXT, NAMESPACE } from '../harness/paths';

const NAME = 'live-probe';
const UPDATED = 'live-update';

test.afterAll(() => {
  kubectl(['-n', NAMESPACE, 'delete', 'configmap', NAME, '--ignore-not-found']);
  kubectl(['-n', NAMESPACE, 'delete', 'configmap', UPDATED, '--ignore-not-found']);
});

test('an object created in the cluster arrives without a reload', async ({ page }) => {
  kubectl(['-n', NAMESPACE, 'delete', 'configmap', NAME, '--ignore-not-found']);
  await openResource(page, 'configmaps', 'ConfigMap');
  const rows = page.locator('main tbody tr');
  await expect(rows.filter({ hasText: 'config-sample' }).first()).toBeVisible({ timeout: 60_000 });
  await expect(rows.filter({ hasText: NAME })).toHaveCount(0);
  try {
    kubectl(['-n', NAMESPACE, 'create', 'configmap', NAME, '--from-literal=seen=yes']);
    await expect(rows.filter({ hasText: NAME }).first()).toBeVisible({ timeout: 60_000 });
  } finally {
    kubectl(['-n', NAMESPACE, 'delete', 'configmap', NAME, '--ignore-not-found']);
  }
});

test('an object deleted in the cluster leaves without a reload', async ({ page }) => {
  kubectl(['-n', NAMESPACE, 'delete', 'configmap', NAME, '--ignore-not-found']);
  kubectl(['-n', NAMESPACE, 'create', 'configmap', NAME, '--from-literal=seen=yes']);
  try {
    await openResource(page, 'configmaps', 'ConfigMap');
    const rows = page.locator('main tbody tr');
    await expect(rows.filter({ hasText: NAME }).first()).toBeVisible({ timeout: 60_000 });
    kubectl(['-n', NAMESPACE, 'delete', 'configmap', NAME]);
    await expect(rows.filter({ hasText: NAME })).toHaveCount(0, { timeout: 60_000 });
  } finally {
    kubectl(['-n', NAMESPACE, 'delete', 'configmap', NAME, '--ignore-not-found']);
  }
});

test('a replica change reaches the table it is shown in', async ({ page }) => {
  const original = Number(
    kubectl([
      '-n',
      NAMESPACE,
      'get',
      'deployment/chatty',
      '-o',
      'jsonpath={.spec.replicas}',
    ]).trim(),
  );
  expect(Number.isInteger(original)).toBe(true);
  const changed = original + 1;
  await page.goto(
    authed(`#context=${CONTEXT}&group=apps&version=v1&resource=deployments&kind=Deployment`),
  );
  await page.waitForFunction(() => document.title.startsWith('deployments'), null, {
    timeout: 60_000,
  });
  const row = page.locator('main tbody tr').filter({ hasText: 'chatty' }).first();
  await expect(row).toBeVisible({ timeout: 60_000 });
  try {
    kubectl(['-n', NAMESPACE, 'scale', 'deployment/chatty', `--replicas=${String(changed)}`]);
    await expect(row).toContainText(String(changed), { timeout: 90_000 });
  } finally {
    kubectl(['-n', NAMESPACE, 'scale', 'deployment/chatty', `--replicas=${String(original)}`]);
    await expect(row).toContainText(String(original), { timeout: 90_000 });
  }
});

test('an open object follows a cluster update without a reload', async ({ page }) => {
  kubectl(['-n', NAMESPACE, 'delete', 'configmap', UPDATED, '--ignore-not-found']);
  kubectl(['-n', NAMESPACE, 'create', 'configmap', UPDATED, '--from-literal=state=before']);
  try {
    await openResource(page, 'configmaps', 'ConfigMap');
    await selectRow(page, UPDATED);
    await expect(page).toHaveTitle(new RegExp(`^${UPDATED} configmaps `));
    await page.getByRole('tab', { name: 'Overview', exact: true }).click();
    const value = page.getByRole('textbox', { name: 'state' });
    await expect(value).toHaveValue('before', { timeout: 60_000 });
    kubectl([
      '-n',
      NAMESPACE,
      'patch',
      `configmap/${UPDATED}`,
      '--type=merge',
      '-p',
      '{"data":{"state":"after"}}',
    ]);
    await expect(value).toHaveValue('after', { timeout: 60_000 });
  } finally {
    kubectl(['-n', NAMESPACE, 'delete', 'configmap', UPDATED, '--ignore-not-found']);
  }
});

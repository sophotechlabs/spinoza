import { expect, test } from '../harness/test';
import { openResource } from '../harness/app';
import { authed } from '../harness/test';
import { kubectl } from '../harness/cluster';
import { CONTEXT, NAMESPACE } from '../harness/paths';

const NAME = 'live-probe';

test.afterAll(() => {
  kubectl(['-n', NAMESPACE, 'delete', 'configmap', NAME, '--ignore-not-found']);
});

test('an object created in the cluster arrives without a reload', async ({ page }) => {
  await openResource(page, 'configmaps', 'ConfigMap');
  const rows = page.locator('main tbody tr');
  await expect(rows.filter({ hasText: 'config-sample' }).first()).toBeVisible({ timeout: 60_000 });
  await expect(rows.filter({ hasText: NAME })).toHaveCount(0);
  kubectl(['-n', NAMESPACE, 'create', 'configmap', NAME, '--from-literal=seen=yes']);
  await expect(rows.filter({ hasText: NAME }).first()).toBeVisible({ timeout: 60_000 });
});

test('an object deleted in the cluster leaves without a reload', async ({ page }) => {
  await openResource(page, 'configmaps', 'ConfigMap');
  const rows = page.locator('main tbody tr');
  await expect(rows.filter({ hasText: NAME }).first()).toBeVisible({ timeout: 60_000 });
  kubectl(['-n', NAMESPACE, 'delete', 'configmap', NAME]);
  await expect(rows.filter({ hasText: NAME })).toHaveCount(0, { timeout: 60_000 });
});

test('a replica change reaches the table it is shown in', async ({ page }) => {
  await page.goto(authed(`#context=${CONTEXT}&group=apps&version=v1&resource=deployments&kind=Deployment`));
  await page.waitForFunction(() => document.title.startsWith('deployments'), null, { timeout: 60_000 });
  const row = page.locator('main tbody tr').filter({ hasText: 'chatty' }).first();
  await expect(row).toBeVisible({ timeout: 60_000 });
  kubectl(['-n', NAMESPACE, 'scale', 'deployment/chatty', '--replicas=2']);
  await expect(row).toContainText('2', { timeout: 90_000 });
  kubectl(['-n', NAMESPACE, 'scale', 'deployment/chatty', '--replicas=1']);
  await expect(row).toContainText('1', { timeout: 90_000 });
});

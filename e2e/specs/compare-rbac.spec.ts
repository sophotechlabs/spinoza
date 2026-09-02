import { expect, test } from '../harness/test';
import { openHome, openResource, openView, selectRow } from '../harness/app';
import {
  kubectl,
  kubectlSecond,
  kubectlSecondApply,
  kubectlSecondSoft,
  kubectlSoft,
} from '../harness/cluster';
import { ensurePrimaryActive, openSecondCluster } from '../harness/multicluster';
import { NAMESPACE, SECOND_CONTEXT } from '../harness/paths';

const COMPARED = 'compare-sample';

test.afterAll(() => {
  kubectlSoft(['-n', NAMESPACE, 'delete', 'configmap', COMPARED, '--ignore-not-found']);
  kubectlSecondSoft(['-n', NAMESPACE, 'delete', 'configmap', COMPARED, '--ignore-not-found']);
});

function createComparedObjects(): void {
  kubectlSoft(['-n', NAMESPACE, 'delete', 'configmap', COMPARED, '--ignore-not-found']);
  kubectl(['-n', NAMESPACE, 'create', 'configmap', COMPARED, '--from-literal=side=primary']);
  const namespace = kubectlSecond([
    'create',
    'namespace',
    NAMESPACE,
    '--dry-run=client',
    '-o',
    'yaml',
  ]);
  kubectlSecondApply(namespace);
  kubectlSecondSoft(['-n', NAMESPACE, 'delete', 'configmap', COMPARED, '--ignore-not-found']);
  kubectlSecond([
    '-n',
    NAMESPACE,
    'create',
    'configmap',
    COMPARED,
    '--from-literal=side=secondary',
  ]);
}

test('the permission index names subjects and where their grants apply', async ({ page }) => {
  await openView(page, 'rbac');
  await expect(page.getByLabel('Filter subjects')).toBeVisible({ timeout: 90_000 });
  await expect(page.getByText(/subjects$/).first()).toBeVisible();
  await page.getByLabel('Filter subjects').fill('readonly');
  const readonly = page.getByText(/readonly/i).first();
  await expect(readonly).toBeVisible();
  await expect(page.getByText(/ServiceAccount/).first()).toBeVisible();
});

test('asking who can read pods returns concrete grants', async ({ page }) => {
  await openView(page, 'rbac');
  await page.getByRole('textbox', { name: 'Verb', exact: true }).fill('get');
  await page.getByRole('textbox', { name: 'Resource', exact: true }).fill('pods');
  await page.getByRole('textbox', { name: 'Namespace', exact: true }).fill(NAMESPACE);
  await page.getByRole('button', { name: 'Ask', exact: true }).click();
  await expect(page.getByText(/ can$/).first()).toBeVisible({ timeout: 90_000 });
  await expect(page.getByRole('button', { name: 'Everyone', exact: true })).toBeEnabled();
});

test('wildcard grants cover a resource the cluster does not know', async ({ page }) => {
  await openView(page, 'rbac');
  await page.getByRole('textbox', { name: 'Verb', exact: true }).fill('teleport');
  await page.getByRole('textbox', { name: 'Resource', exact: true }).fill('planets');
  await page.getByRole('button', { name: 'Ask', exact: true }).click();
  await expect(page.getByText('system:masters', { exact: true })).toBeVisible({ timeout: 90_000 });
  await expect(page.getByText('cluster-admin', { exact: true }).first()).toBeVisible();
});

test('a permission question sends every scope field and can be reset', async ({ page }) => {
  await openView(page, 'rbac');
  const form = page.getByText('Who can', { exact: true }).locator('..');
  const ask = form.getByRole('button', { name: 'Ask', exact: true });
  await expect(ask).toBeDisabled();
  await form.getByLabel('Verb').fill('get');
  await expect(ask).toBeDisabled();
  await form.getByLabel('Resource').fill('deployments');
  await form.getByLabel('API group').fill('apps');
  await form.getByLabel('Namespace').fill(NAMESPACE);
  await expect(ask).toBeEnabled();
  const sent = page.waitForRequest((request) => {
    const url = new URL(request.url());
    return (
      url.pathname === '/api/rbac/who' &&
      url.searchParams.get('verb') === 'get' &&
      url.searchParams.get('resource') === 'deployments' &&
      url.searchParams.get('group') === 'apps' &&
      url.searchParams.get('namespace') === NAMESPACE
    );
  });
  await ask.click();
  await sent;
  await expect(page.getByText(/ can$/).first()).toBeVisible({ timeout: 90_000 });
  await form.getByRole('button', { name: 'Everyone', exact: true }).click();
  await expect(form.getByLabel('Verb')).toHaveValue('');
  await expect(form.getByLabel('Resource')).toHaveValue('');
  await expect(page.getByText(/ subjects$/).first()).toBeVisible();
});

test('a subject expands into the binding, role, and concrete rules behind it', async ({ page }) => {
  await openView(page, 'rbac');
  await page.getByLabel('Filter subjects').fill('readonly');
  const grants = page.getByRole('button', {
    name: /Show what .*readonly.* is bound to/,
  });
  await expect(grants).toBeVisible({ timeout: 90_000 });
  await grants.click();
  const main = page.locator('main');
  await expect(main).toContainText(
    'ClusterRoleBinding spinoza-e2e-readonly → ClusterRole spinoza-e2e-readonly · everywhere',
  );
  await expect(main).toContainText('get, list, watch on pods, services, configmaps');
});

test('compare explains that a second context is required', async ({ page }) => {
  await openResource(page, 'configmaps', 'ConfigMap');
  await selectRow(page, 'config-sample');
  await page.getByRole('tab', { name: 'Compare', exact: true }).click();
  await expect(page.getByText(/Comparing needs a second context/)).toBeVisible();
});

test('object comparison reads both clusters and renders their actual difference', async ({
  page,
}) => {
  createComparedObjects();

  await openHome(page);
  await openSecondCluster(page);
  await ensurePrimaryActive(page);
  await openResource(page, 'configmaps', 'ConfigMap');
  await selectRow(page, COMPARED);
  await page.getByRole('tab', { name: 'Compare', exact: true }).click();
  await page.getByLabel('Against').selectOption({ label: SECOND_CONTEXT });
  await page.getByRole('button', { name: 'Compare', exact: true }).click();
  await expect(page.getByText(/lines differ/)).toBeVisible({ timeout: 90_000 });
  const panel = page.getByRole('tabpanel', { name: 'Compare' });
  await expect(panel).toContainText('primary');
  await expect(panel).toContainText('secondary');
});

test('an object missing from the other cluster is reported as missing, not empty', async ({
  page,
}) => {
  await openHome(page);
  await openSecondCluster(page);
  await ensurePrimaryActive(page);
  await openResource(page, 'configmaps', 'ConfigMap');
  await selectRow(page, 'config-sample');
  await page.getByRole('tab', { name: 'Compare', exact: true }).click();
  const panel = page.getByRole('tabpanel', { name: 'Compare' });
  await panel.getByLabel('Against').selectOption({ label: SECOND_CONTEXT });
  await panel.getByRole('button', { name: 'Compare', exact: true }).click();
  await expect(panel).toContainText('that context has no such object', { timeout: 90_000 });
});

test('show everything sends a raw comparison request', async ({ page }) => {
  createComparedObjects();
  await openHome(page);
  await openSecondCluster(page);
  await ensurePrimaryActive(page);
  await openResource(page, 'configmaps', 'ConfigMap');
  await selectRow(page, COMPARED);
  await page.getByRole('tab', { name: 'Compare', exact: true }).click();
  const panel = page.getByRole('tabpanel', { name: 'Compare' });
  await panel.getByLabel('Against').selectOption({ label: SECOND_CONTEXT });
  await panel.getByRole('checkbox', { name: 'Show everything' }).check();
  const raw = page.waitForRequest((request) => {
    const url = new URL(request.url());
    return url.pathname === '/api/compare' && url.searchParams.get('raw') === 'true';
  });
  await panel.getByRole('button', { name: 'Compare', exact: true }).click();
  await raw;
  await expect(panel).toContainText(/lines differ/, { timeout: 90_000 });
});

test('kind comparison reports objects missing from the other cluster', async ({ page }) => {
  await openHome(page);
  await openSecondCluster(page);
  await ensurePrimaryActive(page);
  await openResource(page, 'configmaps', 'ConfigMap');
  await page.getByRole('tab', { name: 'Compare', exact: true }).click();
  const panel = page.getByRole('tabpanel', { name: 'Compare' });
  await panel.getByLabel('Against').selectOption({ label: SECOND_CONTEXT });
  await panel.getByLabel('What to compare').selectOption('kind');
  await panel.getByRole('button', { name: 'Compare every ConfigMap', exact: true }).click();
  const config = panel.locator('tbody tr').filter({ hasText: 'config-sample' });
  await expect(config).toBeVisible({ timeout: 90_000 });
  await expect(config).toContainText('only here');
});

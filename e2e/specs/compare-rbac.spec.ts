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
  await page.getByLabel('Verb').fill('get');
  await page.getByLabel('Resource').fill('pods');
  await page.getByLabel('Namespace').fill(NAMESPACE);
  await page.getByRole('button', { name: 'Ask', exact: true }).click();
  await expect(page.getByText(/ can$/).first()).toBeVisible({ timeout: 90_000 });
  await expect(page.getByRole('button', { name: 'Everyone', exact: true })).toBeEnabled();
});

test('an impossible permission question answers nobody', async ({ page }) => {
  await openView(page, 'rbac');
  await page.getByLabel('Verb').fill('teleport');
  await page.getByLabel('Resource').fill('planets');
  await page.getByRole('button', { name: 'Ask', exact: true }).click();
  await expect(page.getByText('Nobody.', { exact: true })).toBeVisible({ timeout: 90_000 });
});

test('compare explains that a second context is required', async ({ page }) => {
  await openResource(page, 'configmaps', 'ConfigMap');
  await selectRow(page, 'config-sample');
  await page.getByRole('tab', { name: 'Compare', exact: true }).click();
  await expect(page.getByText(/Comparing needs a second context/)).toBeVisible();
});

test('object comparison reads both clusters and renders their actual difference', async ({ page }) => {
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

test('kind comparison reports objects missing from the other cluster', async ({ page }) => {
  await openHome(page);
  await openSecondCluster(page);
  await ensurePrimaryActive(page);
  await openResource(page, 'configmaps', 'ConfigMap');
  await page.getByRole('tab', { name: 'Compare', exact: true }).click();
  await page.getByLabel('Against').selectOption({ label: SECOND_CONTEXT });
  await page.getByLabel('What to compare').selectOption('kind');
  await expect(page.getByText(/config-sample/).first()).toBeVisible({ timeout: 90_000 });
});

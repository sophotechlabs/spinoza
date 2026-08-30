import { mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { authed, expect, test } from '../harness/test';
import { openGrouped, openHome, openResource, openView, selectRow } from '../harness/app';
import { kubectl, kubectlSoft } from '../harness/cluster';
import { CONTEXT, E2E_DIR, SECOND_CONTEXT, SECOND_KUBECONFIG } from '../harness/paths';
import type { Page } from '@playwright/test';

const OUT = join(E2E_DIR, 'shots', 'out');
const FIXTURES = join(E2E_DIR, 'fixtures', 'shots');

const SEEDED = [
  'namespaces.yaml',
  'storefront.yaml',
  'payments.yaml',
  'platform.yaml',
  'broken.yaml',
];

test.describe.configure({ mode: 'serial' });

const THEME = 'Borg';

test.beforeAll(() => {
  mkdirSync(OUT, { recursive: true });
  for (const file of SEEDED) {
    kubectl(['apply', '-f', join(FIXTURES, file)]);
  }
  for (const target of ['storefront/web', 'payments/ledger', 'platform/gateway']) {
    const [namespace, name] = target.split('/');
    kubectlSoft([
      '-n',
      namespace,
      'rollout',
      'status',
      `deployment/${name}`,
      '--timeout=180s',
    ]);
  }
});

test.afterAll(() => {
  for (const namespace of ['storefront', 'payments', 'platform', 'observability']) {
    kubectlSoft(['delete', 'namespace', namespace, '--ignore-not-found', '--timeout=180s']);
  }
});

async function settle(page: Page): Promise<void> {
  await page.waitForLoadState('networkidle').catch(() => undefined);
  await page.waitForTimeout(1_500);
}

async function shoot(page: Page, name: string): Promise<void> {
  await settle(page);
  await page.screenshot({ path: join(OUT, `${name}.png`) });
}

test('every shot is taken in the same theme', async ({ page }) => {
  await openHome(page);
  await page.getByRole('button', { name: 'Settings' }).click();
  await expect(page.getByRole('dialog', { name: 'Settings' })).toBeVisible({
    timeout: 60_000,
  });
  await page
    .getByRole('combobox', { name: 'Theme preference' })
    .selectOption({ label: THEME });
  await expect(
    page.getByRole('combobox', { name: 'Theme preference' }),
  ).toHaveValue('borg');
  await page.getByRole('button', { name: 'Close settings' }).click().catch(() => undefined);
  await page.keyboard.press('Escape');
});

test('the flux overview', async ({ page }) => {
  kubectl(['apply', '-f', join(FIXTURES, 'flux.yaml')]);
  kubectl(['-n', 'flux-system', 'get', 'gitrepository/podinfo']);
  kubectl(['-n', 'flux-system', 'get', 'kustomization/podinfo']);
  kubectlSoft([
    '-n',
    'flux-system',
    'wait',
    '--for=condition=Ready',
    'gitrepository/podinfo',
    '--timeout=180s',
  ]);
  kubectlSoft([
    '-n',
    'flux-system',
    'wait',
    '--for=condition=Ready',
    'kustomization/podinfo',
    '--timeout=180s',
  ]);
  await openHome(page);
  await page
    .getByRole('button', { name: 'Reconnect' })
    .click({ timeout: 10_000 })
    .catch(() => undefined);
  await page.waitForTimeout(2_000);
  await page.reload();
  await page.waitForLoadState('domcontentloaded');
  await openView(page, 'flux-roles');
  await expect(page.locator('main')).toContainText('All systems operational', {
    timeout: 120_000,
  });
  await expect(page.locator('main')).toContainText('1/1 ready', { timeout: 120_000 });
  await page.waitForTimeout(2_000);
  await shoot(page, 'flux-overview');
});

test('the gitops dependency graph', async ({ page }) => {
  await openView(page, 'gitops');
  await expect(page.locator('main')).toContainText('podinfo', { timeout: 120_000 });
  await page.waitForTimeout(3_000);
  await shoot(page, 'gitops-graph');
});

test('cluster overview', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('group', { name: 'Cluster overview' })).toBeVisible({
    timeout: 90_000,
  });
  await shoot(page, 'cluster-overview');
});

test('every pod in the cluster', async ({ page }) => {
  await openResource(page, 'pods', 'Pod');
  await expect(page.locator('main tbody tr').first()).toBeVisible({ timeout: 90_000 });
  await shoot(page, 'pods');
});

test('what is broken right now', async ({ page }) => {
  await openView(page, 'issues');
  await expect(page.locator('main')).toBeVisible({ timeout: 90_000 });
  await shoot(page, 'issues');
});

test('best-practice checks', async ({ page }) => {
  await openView(page, 'checks');
  await expect(page.locator('main')).toBeVisible({ timeout: 90_000 });
  await page.waitForTimeout(4_000);
  await shoot(page, 'checks');
});

test('the ownership graph', async ({ page }) => {
  await page.goto(
    authed(
      `#context=${CONTEXT}&view=topology&group=apps&version=v1&resource=deployments` +
        '&kind=Deployment&namespace=storefront&name=web',
    ),
  );
  await page.waitForLoadState('domcontentloaded');
  await expect(page.locator('.react-flow__node').first()).toBeVisible({ timeout: 90_000 });
  await page.waitForTimeout(3_000);
  await page
    .getByRole('button', { name: /fit view/i })
    .click({ timeout: 10_000 })
    .catch(() => undefined);
  await page.waitForTimeout(2_000);
  await shoot(page, 'topology');
});

test('helm releases', async ({ page }) => {
  await openView(page, 'helm');
  await expect(page.locator('main')).toBeVisible({ timeout: 90_000 });
  await shoot(page, 'helm-releases');
});

test('forwarding a port', async ({ page }) => {
  await openResource(page, 'pods', 'Pod');
  const row = page
    .locator('main tbody tr')
    .filter({ hasText: 'healthy-' })
    .filter({ hasText: 'Running' })
    .first();
  await expect(row).toBeVisible({ timeout: 90_000 });
  await row.click();
  await page.getByRole('tab', { name: 'Overview' }).click();
  await page.getByRole('button', { name: 'Forward', exact: true }).click();
  await page.getByRole('tab', { name: 'Forwards', exact: true }).click();
  await expect(page.getByRole('tabpanel', { name: 'Forwards' })).toContainText('healthy-', {
    timeout: 60_000,
  });
  await shoot(page, 'port-forward');
});

test('what a release put in the cluster', async ({ page }) => {
  await openView(page, 'helm');
  const row = page.locator('main tbody tr').filter({ hasText: 'e2e-release' }).first();
  await expect(row).toBeVisible({ timeout: 90_000 });
  await row.getByRole('button', { name: 'e2e-release', exact: true }).click();
  const tab = page.getByRole('tab', { name: 'Release', exact: true });
  await expect(tab).toBeEnabled({ timeout: 60_000 });
  await tab.click();
  const panel = page.getByRole('tabpanel', { name: 'Release' });
  await expect(panel).toBeVisible({ timeout: 90_000 });
  const chip = panel.getByRole('button', { name: 'Resources', exact: true });
  await chip.click();
  await expect(chip).toHaveAttribute('aria-pressed', 'true');
  await shoot(page, 'helm-resources');
});

test('the plan before a drain runs', async ({ page }) => {
  const node = kubectl([
    'get',
    'nodes',
    '-l',
    'spinoza.test/pool=drain',
    '-o',
    'jsonpath={.items[0].metadata.name}',
  ]).trim();
  await openResource(page, 'nodes', 'Node');
  await selectRow(page, node);
  await page.getByRole('tab', { name: 'Overview', exact: true }).click();
  await page.getByRole('button', { name: 'Drain', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Drain now', exact: true })).toBeVisible({
    timeout: 60_000,
  });
  await shoot(page, 'drain-plan');
});

test('two clusters side by side', async ({ page }) => {
  await openHome(page);
  const opened = await page.evaluate(
    async ([path, name]) => {
      await fetch(`/api/kubeconfigs?path=${encodeURIComponent(path)}`, { method: 'POST' });
      const query = `name=${encodeURIComponent(name)}&kubeconfig=${encodeURIComponent(path)}`;
      const response = await fetch(`/api/clusters?${query}`, { method: 'POST' });
      return response.status;
    },
    [SECOND_KUBECONFIG, SECOND_CONTEXT],
  );
  expect(opened).toBeLessThan(400);
  await page.reload();
  await page.waitForLoadState('domcontentloaded');
  await openResource(page, 'configmaps', 'ConfigMap');
  await selectRow(page, 'storefront-config');
  const compare = page.getByRole('tab', { name: 'Compare', exact: true });
  await expect(compare).toBeEnabled({ timeout: 60_000 });
  await compare.click();
  await page.waitForTimeout(3_000);
  await shoot(page, 'compare');
});

test('what spinoza changed', async ({ page }) => {
  for (const workload of ['chatty', 'healthy', 'chatty']) {
    await openGrouped(page, 'apps', 'deployments', 'Deployment');
    await selectRow(page, workload);
    await page.getByRole('tab', { name: 'Overview', exact: true }).click();
    await page.getByRole('button', { name: 'Restart', exact: true }).click();
    await page
      .getByRole('button', { name: 'Confirm', exact: true })
      .first()
      .click({ timeout: 5_000 })
      .catch(() => undefined);
    await page.waitForTimeout(1_000);
  }
  await openView(page, 'history');
  await expect(page.locator('main')).toBeVisible({ timeout: 90_000 });
  await shoot(page, 'history');
});

test('inspecting a live object', async ({ page }) => {
  await openResource(page, 'configmaps', 'ConfigMap');
  await selectRow(page, 'storefront-config');
  await page.getByRole('tab', { name: 'YAML', exact: true }).click();
  await expect(page.locator('.monaco-editor').first()).toBeVisible({ timeout: 90_000 });
  await shoot(page, 'inspect-yaml');
});

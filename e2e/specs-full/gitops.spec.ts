import { expect, test } from '../harness/test';
import { openView } from '../harness/app';
import { kubectl, kubectlApply, kubectlSoft } from '../harness/cluster';
import type { Locator, Page } from '@playwright/test';

async function withManagedService(name: string, run: () => Promise<void>): Promise<void> {
  try {
    kubectlApply(JSON.stringify({ apiVersion: 'v1', kind: 'Namespace', metadata: { name } }));
    kubectlApply(
      JSON.stringify({
        apiVersion: 'kustomize.toolkit.fluxcd.io/v1',
        kind: 'Kustomization',
        metadata: { name, namespace: 'flux-system' },
        spec: {
          interval: '10m',
          targetNamespace: name,
          prune: true,
          sourceRef: { kind: 'GitRepository', name: 'podinfo' },
          path: './kustomize',
          timeout: '2m',
        },
      }),
    );
    kubectl([
      'wait',
      '-n',
      'flux-system',
      `kustomization/${name}`,
      '--for=condition=Ready',
      '--timeout=120s',
    ]);
    kubectl([
      'patch',
      '-n',
      'flux-system',
      `kustomization/${name}`,
      '--type=merge',
      '-p',
      '{"spec":{"suspend":true}}',
    ]);
    expect(kubectl(['get', 'service/podinfo', '-n', name, '-o', 'jsonpath={.metadata.name}'])).toBe(
      'podinfo',
    );
    await run();
  } finally {
    kubectlSoft([
      'delete',
      'kustomization',
      name,
      '-n',
      'flux-system',
      '--ignore-not-found',
      '--timeout=60s',
    ]);
    kubectlSoft(['delete', 'namespace', name, '--ignore-not-found', '--wait=false']);
  }
}

async function managedService(page: Page, name: string): Promise<Locator> {
  await openView(page, 'flux-list');
  const row = page
    .locator('main tbody tr')
    .filter({ hasText: 'Kustomization' })
    .filter({ hasText: name });
  await row.getByRole('button', { name, exact: true }).click();
  await page.getByRole('tab', { name: 'Application', exact: true }).click();
  const panel = page.getByRole('tabpanel', { name: 'Application' });
  await expect(panel).toContainText('Nothing will reconcile this');
  const service = panel
    .getByRole('checkbox', { name: 'Mark Service podinfo', exact: true })
    .locator('../..');
  await expect(service).toContainText(name);
  return service;
}

function affinity(name: string, value: string): void {
  kubectl([
    'patch',
    'service/podinfo',
    '-n',
    name,
    '--type=merge',
    '--field-manager=e2e-operator',
    '-p',
    JSON.stringify({ spec: { sessionAffinity: value } }),
  ]);
}

test('GitOps drift compares declared and live values and clears after the resource is repaired', async ({
  page,
}) => {
  test.setTimeout(240_000);
  const name = 'e2e-declared-drift';
  await withManagedService(name, async () => {
    const declaration = JSON.stringify({
      apiVersion: 'v1',
      kind: 'Service',
      metadata: { name: 'podinfo', namespace: name },
      spec: { sessionAffinity: 'None' },
    });
    kubectl([
      'annotate',
      'service/podinfo',
      '-n',
      name,
      '--overwrite',
      `kubectl.kubernetes.io/last-applied-configuration=${declaration}`,
    ]);
    const service = await managedService(page, name);
    await expect(service).not.toContainText('spec.sessionAffinity');
    affinity(name, 'ClientIP');
    await expect(service).toContainText('spec.sessionAffinity None → ClientIP', {
      timeout: 60_000,
    });
    affinity(name, 'None');
    await expect(service).not.toContainText('spec.sessionAffinity', { timeout: 60_000 });
    await service.getByRole('button', { name: 'podinfo', exact: true }).click();
    await expect(page).toHaveTitle(/^podinfo /);
    await expect(page.getByRole('tab', { name: 'YAML', exact: true })).toBeVisible();
  });
});

test('GitOps ownership names the writer that took a server-applied field even when its value is restored', async ({
  page,
}) => {
  test.setTimeout(240_000);
  const name = 'e2e-field-ownership';
  await withManagedService(name, async () => {
    const service = await managedService(page, name);
    await expect(service).toContainText(
      'no spec field is held by anything other than kustomize-controller',
      { timeout: 60_000 },
    );
    affinity(name, 'ClientIP');
    await expect(service).toContainText(
      'spec.sessionAffinity kustomize-controller → e2e-operator',
      { timeout: 60_000 },
    );
    await expect(service).toContainText('this object is applied server-side');
    affinity(name, 'None');
    await expect(service).toContainText('spec.sessionAffinity kustomize-controller → e2e-operator');
    expect(
      kubectl(['get', 'service/podinfo', '-n', name, '-o', 'jsonpath={.spec.sessionAffinity}']),
    ).toBe('None');
  });
});

test('a missing GitOps resource leaves the remaining application resources inspectable', async ({
  page,
}) => {
  test.setTimeout(240_000);
  const name = 'e2e-missing-managed';
  await withManagedService(name, async () => {
    await managedService(page, name);
    kubectl(['delete', 'service/podinfo', '-n', name, '--wait=true']);
    await page.reload();
    const panel = page.getByRole('tabpanel', { name: 'Application' });
    await expect(
      panel.getByRole('checkbox', { name: 'Mark Service podinfo', exact: true }),
    ).toBeVisible();
    const deployment = panel
      .getByRole('checkbox', { name: 'Mark Deployment podinfo', exact: true })
      .locator('../..');
    await deployment.getByRole('button', { name: 'podinfo', exact: true }).click();
    await expect(page).toHaveTitle(/^podinfo /);
    await expect(page.getByRole('tab', { name: 'YAML', exact: true })).toBeVisible();
    expect(
      kubectl(['get', 'deployment/podinfo', '-n', name, '-o', 'jsonpath={.metadata.name}']),
    ).toBe('podinfo');
  });
});

test('the graph draws what manages what, not just a legend', async ({ page }) => {
  await openView(page, 'gitops');
  await expect
    .poll(() => page.locator('.react-flow__node').count(), { timeout: 120_000 })
    .toBeGreaterThan(1);
  await expect
    .poll(() => page.locator('.react-flow__edge').count(), { timeout: 120_000 })
    .toBeGreaterThan(0);
});

test('an edge names the source it comes from and the applier it feeds', async ({ page }) => {
  await openView(page, 'gitops');
  const edge = page.getByRole('group', {
    name: /^Edge from source\.toolkit\.fluxcd\.io\/GitRepository\/.+ to kustomize\.toolkit\.fluxcd\.io\/Kustomization\/.+$/,
  });
  await expect(edge.first()).toBeAttached({ timeout: 120_000 });
});

test('both controllers land in one graph', async ({ page }) => {
  await openView(page, 'gitops');
  const main = page.locator('main');
  await expect(main).toContainText('podinfo', { timeout: 120_000 });
  await expect(main).toContainText('guestbook');
});

test('the graph says what its colours and edges mean', async ({ page }) => {
  await openView(page, 'gitops');
  const main = page.locator('main');
  await expect(main).toContainText('Manages', { timeout: 120_000 });
  await expect(main).toContainText('Depends on');
  await expect(main).toContainText('Source, not ready yet');
});

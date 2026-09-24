import { expect, test } from '../harness/test';
import { openView } from '../harness/app';
import { kubectl, kubectlApply, kubectlSoft } from '../harness/cluster';
import type { Locator, Page } from '@playwright/test';

interface ManagedApplication {
  spec: {
    source: Record<string, unknown>;
    syncPolicy?: { automated?: { enabled: boolean; prune: boolean; selfHeal: boolean } };
  };
  status?: {
    sync?: { revision?: string };
    resources?: { kind: string; name: string; namespace: string }[];
    operationState?: {
      phase: string;
      message?: string;
      operation: {
        sync: { dryRun?: boolean; resources?: { kind: string; name: string; namespace: string }[] };
      };
    };
  };
}

function namedApplication(name: string): ManagedApplication {
  return JSON.parse(
    kubectl(['get', `application/${name}`, '-n', 'argocd', '-o', 'json']),
  ) as ManagedApplication;
}

async function withApplication(page: Page, name: string, run: () => Promise<void>): Promise<void> {
  const original = namedApplication('guestbook');
  const revision = original.status?.sync?.revision;
  expect(revision).toBeTruthy();
  try {
    kubectlApply(JSON.stringify({ apiVersion: 'v1', kind: 'Namespace', metadata: { name } }));
    kubectlApply(
      JSON.stringify({
        apiVersion: 'argoproj.io/v1alpha1',
        kind: 'Application',
        metadata: { name, namespace: 'argocd' },
        spec: {
          project: 'default',
          source: { ...original.spec.source, targetRevision: revision },
          destination: { server: 'https://kubernetes.default.svc', namespace: name },
          syncPolicy: { automated: { enabled: false, prune: true, selfHeal: true } },
        },
      }),
    );
    await expect
      .poll(() => namedApplication(name).status?.resources?.length ?? 0, { timeout: 120_000 })
      .toBeGreaterThan(0);
    await openView(page, 'argo-apps');
    await page.getByRole('button', { name: new RegExp(`^${name} `) }).click();
    await page.getByRole('tab', { name: 'Overview', exact: true }).click();
    await run();
  } finally {
    kubectlSoft([
      'delete',
      'application',
      name,
      '-n',
      'argocd',
      '--ignore-not-found',
      '--wait=true',
    ]);
    kubectlSoft(['delete', 'namespace', name, '--ignore-not-found', '--wait=false']);
  }
}

test('an Argo dry run leaves resources absent and a selected sync creates only the marked Service', async ({
  page,
}) => {
  test.setTimeout(240_000);
  const name = 'e2e-selected-sync';
  await withApplication(page, name, async () => {
    const service = namedApplication(name).status?.resources?.find(
      (resource) => resource.kind === 'Service',
    );
    if (service === undefined) {
      throw new Error('the fixture application has no Service to synchronize');
    }
    await page.getByRole('button', { name: 'Sync', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: `Sync ${name}`, exact: true });
    await dialog.getByRole('checkbox', { name: /^Dry run/ }).check();
    await dialog.getByRole('button', { name: 'Synchronize', exact: true }).click();
    await expect(dialog).toBeHidden();
    await expect
      .poll(() => namedApplication(name).status?.operationState, { timeout: 120_000 })
      .toMatchObject({ phase: 'Succeeded', operation: { sync: { dryRun: true } } });
    expect(kubectl(['get', 'services,deployments', '-n', name, '-o', 'name'])).toBe('');
    await page.getByRole('tab', { name: 'Application', exact: true }).click();
    const panel = page.getByRole('tabpanel', { name: 'Application' });
    await panel
      .getByRole('checkbox', { name: `Mark Service ${service.name}`, exact: true })
      .check();
    await panel.getByRole('button', { name: 'Sync 1 marked', exact: true }).click();
    await expect(panel).toContainText('Sync requested.');
    await expect
      .poll(() => namedApplication(name).status?.operationState, { timeout: 120_000 })
      .toMatchObject({
        phase: 'Succeeded',
        operation: {
          sync: { resources: [{ kind: 'Service', name: service.name, namespace: name }] },
        },
      });
    expect(kubectl(['get', 'services', '-n', name, '-o', 'name']).trim()).toBe(
      `service/${service.name}`,
    );
    expect(kubectl(['get', 'deployments', '-n', name, '-o', 'name'])).toBe('');
    const row = panel
      .getByRole('checkbox', { name: `Mark Service ${service.name}`, exact: true })
      .locator('../..');
    await expect(row).toContainText('Synced', { timeout: 60_000 });
  });
});

test('a controller refusal leaves a selected Service absent and the same sync recovers after quota repair', async ({
  page,
}) => {
  test.setTimeout(300_000);
  const name = 'e2e-sync-recovery';
  await withApplication(page, name, async () => {
    const service = namedApplication(name).status?.resources?.find(
      (resource) => resource.kind === 'Service',
    );
    if (service === undefined) {
      throw new Error('the fixture application has no Service to synchronize');
    }
    kubectlApply(
      JSON.stringify({
        apiVersion: 'v1',
        kind: 'ResourceQuota',
        metadata: { name: 'refuse-services', namespace: name },
        spec: { hard: { services: '0' } },
      }),
    );
    await expect
      .poll(() =>
        kubectl([
          'get',
          'resourcequota',
          'refuse-services',
          '-n',
          name,
          '-o',
          'jsonpath={.status.hard.services}',
        ]),
      )
      .toBe('0');
    await page.getByRole('tab', { name: 'Application', exact: true }).click();
    const panel = page.getByRole('tabpanel', { name: 'Application' });
    const mark = panel.getByRole('checkbox', { name: `Mark Service ${service.name}`, exact: true });
    await mark.check();
    await panel.getByRole('button', { name: 'Sync 1 marked', exact: true }).click();
    await expect(panel).toContainText('Sync requested.');
    await expect
      .poll(() => namedApplication(name).status?.operationState, { timeout: 120_000 })
      .toMatchObject({
        phase: 'Failed',
        message: expect.stringContaining('exceeded quota'),
        operation: {
          sync: { resources: [{ kind: 'Service', name: service.name, namespace: name }] },
        },
      });
    expect(kubectl(['get', 'services,deployments', '-n', name, '-o', 'name'])).toBe('');
    kubectl(['delete', 'resourcequota', 'refuse-services', '-n', name, '--wait=true']);
    await mark.check();
    await panel.getByRole('button', { name: 'Sync 1 marked', exact: true }).click();
    await expect
      .poll(() => namedApplication(name).status?.operationState?.phase, { timeout: 120_000 })
      .toBe('Succeeded');
    expect(kubectl(['get', 'services', '-n', name, '-o', 'name']).trim()).toBe(
      `service/${service.name}`,
    );
    expect(kubectl(['get', 'deployments', '-n', name, '-o', 'name'])).toBe('');
    await expect(mark.locator('../..')).toContainText('Synced', { timeout: 60_000 });
  });
});

test('Argo auto-sync can be resumed and suspended without changing prune or self-heal', async ({
  page,
}) => {
  test.setTimeout(180_000);
  const name = 'e2e-auto-sync';
  await withApplication(page, name, async () => {
    await page.getByRole('button', { name: 'Resume auto-sync', exact: true }).click();
    await expect
      .poll(() => namedApplication(name).spec.syncPolicy?.automated)
      .toEqual({ enabled: true, prune: true, selfHeal: true });
    await expect(page.getByText('Auto-sync on.', { exact: true })).toBeVisible();
    await page.getByRole('button', { name: 'Suspend auto-sync', exact: true }).click();
    await expect
      .poll(() => namedApplication(name).spec.syncPolicy?.automated)
      .toEqual({ enabled: false, prune: true, selfHeal: true });
    await expect(
      page.getByText('Auto-sync off. Prune and self-heal unchanged.', { exact: true }),
    ).toBeVisible();
  });
});

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

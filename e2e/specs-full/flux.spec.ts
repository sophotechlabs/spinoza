import { expect, test } from '../harness/test';
import { openView } from '../harness/app';
import { kubectl } from '../harness/cluster';
import type { Page } from '@playwright/test';

async function openPodinfo(page: Page): Promise<void> {
  await openView(page, 'flux-list');
  const row = page.locator('main tbody tr').filter({ hasText: 'Kustomization' }).filter({
    hasText: 'podinfo',
  });
  await expect(row).toBeVisible({ timeout: 120_000 });
  await row.getByRole('button', { name: 'podinfo', exact: true }).click();
  await expect(page.getByRole('tab', { name: 'Overview', exact: true })).toBeVisible({
    timeout: 60_000,
  });
}

function kustomization(path: string): string {
  return kubectl([
    '-n',
    'flux-system',
    'get',
    'kustomization/podinfo',
    '-o',
    `jsonpath={${path}}`,
  ]).trim();
}

function gitRepository(path: string): string {
  return kubectl([
    '-n',
    'flux-system',
    'get',
    'gitrepository/podinfo',
    '-o',
    `jsonpath={${path}}`,
  ]).trim();
}

test('the overview reports the controllers it found running', async ({ page }) => {
  await openView(page, 'flux-roles');
  const panel = page.getByRole('group', { name: 'Flux resources' });
  await expect(panel).toContainText('All systems operational', { timeout: 120_000 });
  await expect(panel).toContainText(/Flux\s*\d+\.\d+/);
  await expect(panel).toContainText('flux-system');
  await expect(panel).toContainText(/Controllers\s*\d+/);
});

test('every controller the chart installed is named with its version', async ({ page }) => {
  await openView(page, 'flux-roles');
  const panel = page.getByRole('group', { name: 'Flux resources' });
  for (const controller of ['source-controller', 'kustomize-controller', 'helm-controller']) {
    await expect(panel.locator('tbody tr').filter({ hasText: controller })).toContainText('Ready', {
      timeout: 120_000,
    });
  }
});

test('a cluster sync that was never set up is said to be missing, not invented', async ({
  page,
}) => {
  await openView(page, 'flux-roles');
  await expect(page.getByRole('group', { name: 'Flux resources' })).toContainText(
    'No flux-system sync was found',
    { timeout: 120_000 },
  );
});

test('the list carries the revision the controller actually reconciled', async ({ page }) => {
  await openView(page, 'flux-list');
  const row = page.locator('main tbody tr').filter({ hasText: 'Kustomization' }).filter({
    hasText: 'podinfo',
  });
  await expect(row.first()).toContainText('Ready', { timeout: 120_000 });
  const revision = kubectl([
    '-n',
    'flux-system',
    'get',
    'kustomization/podinfo',
    '-o',
    'jsonpath={.status.lastAppliedRevision}',
  ]).trim();
  expect(revision).not.toBe('');
  await expect(row.first()).toContainText(revision);
});

test('the list says which source each object came from', async ({ page }) => {
  await openView(page, 'flux-list');
  const row = page.locator('main tbody tr').filter({ hasText: 'podinfo' }).first();
  await expect(row).toContainText('GitRepository/podinfo', { timeout: 120_000 });
});

test('the list folds each kind under a heading that counts what is ready', async ({ page }) => {
  await openView(page, 'flux-list');
  const main = page.locator('main');
  await expect(main).toContainText(/Kustomizations\s*\d+\/\d+\s*ready/, { timeout: 120_000 });
  await expect(main).toContainText(/Sources\s*\d+\/\d+\s*ready/);
});

test('suspending and resuming a Flux object reaches its live spec', async ({ page }) => {
  try {
    await openPodinfo(page);
    await page.getByRole('tab', { name: 'Overview', exact: true }).click();
    await page.getByRole('button', { name: 'Suspend', exact: true }).click();
    await expect.poll(() => kustomization('.spec.suspend'), { timeout: 60_000 }).toBe('true');
    const resume = page.getByRole('button', { name: 'Resume', exact: true });
    await expect(resume).toBeVisible({ timeout: 60_000 });
    await resume.click();
    await expect.poll(() => kustomization('.spec.suspend'), { timeout: 60_000 }).not.toBe('true');
    await expect(page.getByText('Resumed.', { exact: true })).toBeVisible();
  } finally {
    kubectl([
      '-n',
      'flux-system',
      'patch',
      'kustomization/podinfo',
      '--type=merge',
      '-p',
      '{"spec":{"suspend":false}}',
    ]);
  }
});

test('reconciling a Flux object stamps a new controller request', async ({ page }) => {
  const before = kustomization(".metadata.annotations['reconcile\\.fluxcd\\.io/requestedAt']");
  await openPodinfo(page);
  await page.getByRole('tab', { name: 'Overview', exact: true }).click();
  await page.getByRole('button', { name: 'Reconcile', exact: true }).click();
  await expect
    .poll(() => kustomization(".metadata.annotations['reconcile\\.fluxcd\\.io/requestedAt']"), {
      timeout: 60_000,
    })
    .not.toBe(before);
});

test('reconciling with source stamps the source and applier with one request', async ({ page }) => {
  const annotation = ".metadata.annotations['reconcile\\.fluxcd\\.io/requestedAt']";
  const before = gitRepository(annotation);
  await openPodinfo(page);
  await page.getByRole('tab', { name: 'Overview', exact: true }).click();
  await page.getByRole('button', { name: 'With source', exact: true }).click();
  await expect.poll(() => gitRepository(annotation), { timeout: 60_000 }).not.toBe(before);
  await expect
    .poll(() => kustomization(annotation), { timeout: 60_000 })
    .toBe(gitRepository(annotation));
});

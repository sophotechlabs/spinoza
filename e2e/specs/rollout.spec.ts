import { expect, test } from '../harness/test';
import { openGrouped, selectRow } from '../harness/app';
import { kubectl } from '../harness/cluster';
import { NAMESPACE } from '../harness/paths';
import type { Locator, Page } from '@playwright/test';

interface Deployment {
  spec: { template: { spec: { containers: { image: string }[] } } };
}

const WORKLOAD = 'e2e-rollout';

function imageOf(name: string): string {
  const held = JSON.parse(
    kubectl(['-n', NAMESPACE, 'get', 'deployment', name, '-o', 'json']),
  ) as Deployment;
  return held.spec.template.spec.containers[0].image;
}

async function openRevisions(page: Page): Promise<Locator> {
  await openGrouped(page, 'apps', 'deployments', 'Deployment');
  await selectRow(page, WORKLOAD);
  await page.getByRole('tab', { name: 'Revisions', exact: true }).click();
  const panel = page.locator('#panel-body-revisions');
  await panel.waitFor({ state: 'visible', timeout: 60_000 });
  return panel;
}

test.describe('rollout revisions', () => {
  test.beforeAll(() => {
    kubectl([
      '-n',
      NAMESPACE,
      'create',
      'deployment',
      WORKLOAD,
      '--image=registry.k8s.io/pause:3.9',
      '--replicas=1',
    ]);
    kubectl(['-n', NAMESPACE, 'rollout', 'status', `deployment/${WORKLOAD}`, '--timeout=90s']);
    kubectl([
      '-n',
      NAMESPACE,
      'set',
      'image',
      `deployment/${WORKLOAD}`,
      '*=registry.k8s.io/pause:3.10',
    ]);
    kubectl(['-n', NAMESPACE, 'rollout', 'status', `deployment/${WORKLOAD}`, '--timeout=90s']);
  });

  test.afterAll(() => {
    kubectl(['-n', NAMESPACE, 'delete', 'deployment', WORKLOAD, '--ignore-not-found']);
  });

  test('lists what rolled out and marks the one running now', async ({ page }) => {
    const panel = await openRevisions(page);

    await expect(panel.getByText('#2', { exact: true })).toBeVisible({ timeout: 60_000 });
    await expect(panel.getByText('#1', { exact: true })).toBeVisible();
    await expect(panel.getByText('current', { exact: true })).toHaveCount(1);
  });

  test('shows what differs between a revision and the one running now', async ({ page }) => {
    await openRevisions(page);
    await page.getByRole('button', { name: 'Diff' }).first().click();

    await expect(page.getByText(/#1 against #2: \d+ lines differ/)).toBeVisible({
      timeout: 60_000,
    });
  });

  test('puts the workload back to the revision it is asked for', async ({ page }) => {
    expect(imageOf(WORKLOAD)).toContain('pause:3.10');
    const panel = await openRevisions(page);

    await panel.getByRole('button', { name: 'Go back to this' }).first().click();

    await expect.poll(() => imageOf(WORKLOAD), { timeout: 60_000 }).toContain('pause:3.9');
  });

  test('offers no revisions for a kind that keeps none', async ({ page }) => {
    await openGrouped(page, '', 'configmaps', 'ConfigMap');
    const first = page.locator('main tbody tr').first();
    await first.waitFor({ state: 'visible', timeout: 60_000 });
    await first.getByRole('button').first().click();

    const tab = page.getByRole('tab', { name: 'Revisions', exact: true });
    await expect(tab).toHaveAttribute('aria-disabled', 'true', { timeout: 60_000 });
    await expect(tab).toHaveAttribute('title', /Deployment, StatefulSet or DaemonSet/);
  });
});

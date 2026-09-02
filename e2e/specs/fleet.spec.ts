import { expect, test } from '../harness/test';
import { openHome, openView } from '../harness/app';
import {
  clusterID,
  ensurePrimaryActive,
  openedClusters,
  openSecondCluster,
} from '../harness/multicluster';
import { CONTEXT, NAMESPACE, SECOND_CONTEXT } from '../harness/paths';
import { kubectl, kubectlSecond } from '../harness/cluster';

interface Summary {
  nodes: { total: number; ready: number };
  pods: { total: number; running: number };
}

function summary(run: (args: string[]) => string): Summary {
  const nodes = JSON.parse(run(['get', 'nodes', '-o', 'json'])) as {
    items: { status?: { conditions?: { type?: string; status?: string }[] } }[];
  };
  const pods = JSON.parse(run(['get', 'pods', '--all-namespaces', '-o', 'json'])) as {
    items: { status?: { phase?: string } }[];
  };
  return {
    nodes: {
      total: nodes.items.length,
      ready: nodes.items.filter((node) => {
        return node.status?.conditions?.some((condition) => {
          return condition.type === 'Ready' && condition.status === 'True';
        });
      }).length,
    },
    pods: {
      total: pods.items.length,
      running: pods.items.filter((pod) => pod.status?.phase === 'Running').length,
    },
  };
}

async function openFleet(page: import('@playwright/test').Page): Promise<void> {
  await openHome(page);
  await openSecondCluster(page);
  await ensurePrimaryActive(page);
  await openView(page, 'fleet');
}

async function openAcrossClusters(
  page: import('@playwright/test').Page,
  view: string,
): Promise<void> {
  await openHome(page);
  await openSecondCluster(page);
  await ensurePrimaryActive(page);
  await openView(page, view);
}

test('fleet exposes every promised cross-cluster inventory', async ({ page }) => {
  await openFleet(page);
  for (const tab of ['Clusters', 'What is on them', 'Releases', 'Delivery', 'Images']) {
    await expect(page.getByRole('button', { name: tab, exact: true })).toBeVisible();
  }
});

test('fleet overview reports both open clusters and its total row', async ({ page }) => {
  await openFleet(page);
  const table = page.locator('main table');
  await expect(table).toContainText(CONTEXT, { timeout: 90_000 });
  await expect(table).toContainText(SECOND_CONTEXT);
  await expect(table).toContainText('Everything open');
});

test('fleet totals are the sum of the nodes and pods on both live clusters', async ({ page }) => {
  await openFleet(page);
  const primary = summary(kubectl);
  const secondary = summary(kubectlSecond);
  const overview = await page.evaluate(async () => {
    const response = await fetch('/api/overview/fleet');
    if (!response.ok) {
      throw new Error(`fleet overview returned ${response.status}`);
    }
    return response.json() as Promise<{
      clusters: {
        context: string;
        nodes: { total: number; ready: number };
        pods: { total: number; running: number };
      }[];
      nodes: { total: number; ready: number };
      pods: { total: number; running: number };
    }>;
  });
  for (const expected of [
    { context: CONTEXT, summary: primary },
    { context: SECOND_CONTEXT, summary: secondary },
  ]) {
    const cluster = overview.clusters.find((one) => one.context === expected.context);
    if (cluster === undefined) {
      throw new Error(`the fleet overview omitted ${expected.context}`);
    }
    expect(cluster.nodes).toMatchObject(expected.summary.nodes);
    expect(cluster.pods).toMatchObject(expected.summary.pods);
    const row = page.locator('main tbody tr').filter({
      has: page.getByRole('button', { name: expected.context, exact: true }),
    });
    await expect(row).toContainText(
      `${String(expected.summary.nodes.ready)}/${String(expected.summary.nodes.total)}`,
    );
    await expect(row).toContainText(
      `${String(expected.summary.pods.running)}/${String(expected.summary.pods.total)}`,
    );
  }
  expect(overview.nodes.total).toBe(primary.nodes.total + secondary.nodes.total);
  expect(overview.nodes.ready).toBe(primary.nodes.ready + secondary.nodes.ready);
  expect(overview.pods.total).toBe(primary.pods.total + secondary.pods.total);
  expect(overview.pods.running).toBe(primary.pods.running + secondary.pods.running);
});

test('the issue queue merges every open cluster without losing provenance', async ({ page }) => {
  await openAcrossClusters(page, 'issues');
  await expect(page.locator('main')).toContainText('crashing', { timeout: 90_000 });
  const primaryID = clusterID(await openedClusters(page), CONTEXT);
  const requested = page.waitForResponse((response) => {
    return new URL(response.url()).pathname === '/api/issues/fleet';
  });
  await page.getByLabel('Every open cluster').check();
  const response = await requested;
  expect(response.ok()).toBe(true);
  const body = (await response.json()) as {
    rows?: { cluster?: string; object?: { name?: string; namespace?: string } }[];
  };
  if (!Array.isArray(body.rows)) {
    throw new Error('the fleet issue response has no rows');
  }
  const crashing = body.rows.find((row) => {
    return row.object?.name === 'crashing' && row.object.namespace === NAMESPACE;
  });
  if (crashing === undefined) {
    throw new Error('the fleet issue response omitted the seeded crashing deployment');
  }
  expect(crashing.cluster).toBe(primaryID);
  const row = page.locator('main li').filter({ hasText: 'crashing' }).first();
  await expect(row).toBeVisible({ timeout: 90_000 });
  await expect(row).toContainText(CONTEXT);
});

test('the audit merges every open cluster without losing finding provenance', async ({ page }) => {
  await openAcrossClusters(page, 'checks');
  const privileged = page.getByRole('button', { name: /Privileged containers/ });
  await expect(privileged).toBeVisible({ timeout: 90_000 });
  const primaryID = clusterID(await openedClusters(page), CONTEXT);
  const requested = page.waitForResponse((response) => {
    return new URL(response.url()).pathname === '/api/checks/fleet';
  });
  await page.getByLabel('Every open cluster').check();
  const response = await requested;
  expect(response.ok()).toBe(true);
  const body = (await response.json()) as {
    objects?: { cluster?: string; name?: string; namespace?: string }[];
  };
  if (!Array.isArray(body.objects)) {
    throw new Error('the fleet checks response has no objects');
  }
  const risky = body.objects.find((object) => {
    return object.name === 'risky' && object.namespace === NAMESPACE;
  });
  if (risky === undefined) {
    throw new Error('the fleet checks response omitted the seeded risky deployment');
  }
  expect(risky.cluster).toBe(primaryID);
  await privileged.click();
  const finding = page.locator('main li').filter({ hasText: 'risky' }).first();
  await expect(finding).toBeVisible({ timeout: 90_000 });
  await expect(finding).toContainText(CONTEXT);
});

test('fleet inventory counts resource kinds per cluster', async ({ page }) => {
  await openFleet(page);
  await page.getByRole('button', { name: 'What is on them', exact: true }).click();
  await expect(page.locator('main').getByText('deployments', { exact: true })).toBeVisible({
    timeout: 90_000,
  });
  await expect(page.getByText(new RegExp(CONTEXT)).first()).toBeVisible();
});

test('fleet releases preserve the owning cluster and Helm release', async ({ page }) => {
  await openFleet(page);
  await page.getByRole('button', { name: 'Releases', exact: true }).click();
  await expect(page.getByText('e2e-release', { exact: true })).toBeVisible({ timeout: 90_000 });
  await expect(page.getByText(new RegExp(CONTEXT)).first()).toBeVisible();
});

test('fleet delivery combines Flux and Argo without losing cluster provenance', async ({
  page,
}) => {
  await openFleet(page);
  await page.getByRole('button', { name: 'Delivery', exact: true }).click();
  await expect(page.getByText(/Flux|Argo/).first()).toBeVisible({ timeout: 90_000 });
  await expect(page.getByText(new RegExp(CONTEXT)).first()).toBeVisible();
});

test('fleet images report image use and version skew across clusters', async ({ page }) => {
  await openFleet(page);
  await page.getByRole('button', { name: 'Images', exact: true }).click();
  await expect(page.getByText(/busybox/).first()).toBeVisible({ timeout: 90_000 });
  await expect(page.getByText(/pods/).first()).toBeVisible();
});

test('choosing a fleet cluster activates that cluster', async ({ page }) => {
  await openFleet(page);
  await page
    .getByRole('button', { name: new RegExp(SECOND_CONTEXT) })
    .first()
    .click();
  await expect(page.getByRole('banner')).toContainText(SECOND_CONTEXT, { timeout: 90_000 });
});

test('fleet tabs expose which inventory is currently active', async ({ page }) => {
  await openFleet(page);
  const clusters = page.getByRole('button', { name: 'Clusters', exact: true });
  const images = page.getByRole('button', { name: 'Images', exact: true });
  await expect(clusters).toHaveAttribute('aria-current', 'true');
  await expect(images).toHaveAttribute('aria-current', 'false');
  await images.click();
  await expect(images).toHaveAttribute('aria-current', 'true');
  await expect(clusters).toHaveAttribute('aria-current', 'false');
  await expect(page.getByText(/busybox/).first()).toBeVisible({ timeout: 90_000 });
});

test('fleet inventory keeps counts from both clusters separately visible', async ({ page }) => {
  await openFleet(page);
  await page.getByRole('button', { name: 'What is on them', exact: true }).click();
  const deployment = page.locator('main li').filter({ hasText: 'Deployment' }).first();
  await expect(deployment).toBeVisible({ timeout: 90_000 });
  await expect(deployment).toContainText(CONTEXT);
  await expect(deployment).toContainText(SECOND_CONTEXT);
});

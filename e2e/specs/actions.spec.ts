import { expect, test } from '../harness/test';
import { openGrouped, openResource, selectRow } from '../harness/app';
import { kubectl } from '../harness/cluster';
import { NAMESPACE } from '../harness/paths';
import type { Page } from '@playwright/test';

interface Deployment {
  spec: {
    replicas?: number;
    template: { metadata?: { annotations?: Record<string, string> } };
  };
}

interface CronJob {
  spec: { suspend?: boolean };
}

interface JobList {
  items: {
    metadata: { name: string; ownerReferences?: { kind: string; name: string }[] };
  }[];
}

interface PodList {
  items: {
    metadata: { name: string; namespace: string; uid: string };
  }[];
}

interface ActionResult {
  action: string;
  message: string;
  dryRun?: boolean;
  pods?: {
    namespace: string;
    name: string;
    outcome: string;
    reason?: string;
  }[];
}

function field(kind: string, name: string, path: string, namespaced = true): string {
  const args = ['get', `${kind}/${name}`, '-o', `jsonpath={${path}}`];
  if (namespaced) {
    return kubectl(['-n', NAMESPACE, ...args]).trim();
  }
  return kubectl(args).trim();
}

function deployment(name: string): Deployment {
  return JSON.parse(
    kubectl(['-n', NAMESPACE, 'get', `deployment/${name}`, '-o', 'json']),
  ) as Deployment;
}

function scale(name: string, replicas: number): void {
  kubectl(['-n', NAMESPACE, 'scale', `deployment/${name}`, `--replicas=${String(replicas)}`]);
}

function annotations(name: string): Record<string, string> | undefined {
  return deployment(name).spec.template.metadata?.annotations;
}

function restoreAnnotations(name: string, original: Record<string, string> | undefined): void {
  const current = annotations(name);
  if (original === undefined && current === undefined) {
    return;
  }
  let operation: { op: string; path: string; value?: Record<string, string> };
  if (original === undefined) {
    operation = { op: 'remove', path: '/spec/template/metadata/annotations' };
  } else if (current === undefined) {
    operation = { op: 'add', path: '/spec/template/metadata/annotations', value: original };
  } else {
    operation = { op: 'replace', path: '/spec/template/metadata/annotations', value: original };
  }
  kubectl([
    '-n',
    NAMESPACE,
    'patch',
    `deployment/${name}`,
    '--type=json',
    '-p',
    JSON.stringify([operation]),
  ]);
  kubectl(['-n', NAMESPACE, 'rollout', 'status', `deployment/${name}`, '--timeout=120s']);
}

function nightlyJobs(): string[] {
  const jobs = JSON.parse(kubectl(['-n', NAMESPACE, 'get', 'jobs', '-o', 'json'])) as JobList;
  return jobs.items
    .filter((job) =>
      job.metadata.ownerReferences?.some(
        (owner) => owner.kind === 'CronJob' && owner.name === 'nightly',
      ),
    )
    .map((job) => job.metadata.name)
    .sort();
}

function podsOnNode(name: string): PodList['items'] {
  const pods = JSON.parse(
    kubectl([
      'get',
      'pods',
      '--all-namespaces',
      '--field-selector',
      `spec.nodeName=${name}`,
      '-o',
      'json',
    ]),
  ) as PodList;
  return pods.items.sort((left, right) => {
    const a = `${left.metadata.namespace}/${left.metadata.name}`;
    const b = `${right.metadata.namespace}/${right.metadata.name}`;
    return a.localeCompare(b);
  });
}

async function openWorkload(page: Page, name: string): Promise<void> {
  await openGrouped(page, 'apps', 'deployments', 'Deployment');
  await selectRow(page, name);
  await page.getByRole('tab', { name: 'Overview', exact: true }).click();
}

async function press(page: Page, name: string, needsConfirmation = false): Promise<void> {
  await page.getByRole('button', { name, exact: true }).click();
  if (needsConfirmation) {
    await page.getByRole('button', { name: 'Confirm', exact: true }).first().click();
  }
}

async function openNode(page: Page, name: string): Promise<void> {
  await openResource(page, 'nodes', 'Node');
  await selectRow(page, name);
  await page.getByRole('tab', { name: 'Overview', exact: true }).click();
}

test('scaling in the browser moves the apiserver', async ({ page }) => {
  const original = deployment('healthy').spec.replicas;
  expect(original).toBeDefined();
  if (original === undefined) {
    throw new Error('healthy has no replica count');
  }
  const changed = original + 1;
  try {
    await openWorkload(page, 'healthy');
    const replicas = page.getByRole('spinbutton', { name: 'replicas' });
    await expect(replicas).toHaveValue(String(original), { timeout: 30_000 });
    await replicas.fill(String(changed));
    await press(page, 'Scale');
    await expect
      .poll(() => field('deployment', 'healthy', '.spec.replicas'), { timeout: 60_000 })
      .toBe(String(changed));
    await expect(
      page.locator('main tbody tr').filter({ hasText: 'healthy' }).first(),
    ).toContainText(String(changed), { timeout: 60_000 });
  } finally {
    scale('healthy', original);
  }
});

test('scaling back down is the same round trip', async ({ page }) => {
  const original = deployment('healthy').spec.replicas;
  expect(original).toBeDefined();
  if (original === undefined) {
    throw new Error('healthy has no replica count');
  }
  const starting = original + 1;
  try {
    scale('healthy', starting);
    await expect
      .poll(() => field('deployment', 'healthy', '.spec.replicas'), { timeout: 60_000 })
      .toBe(String(starting));
    await openWorkload(page, 'healthy');
    const replicas = page.getByRole('spinbutton', { name: 'replicas' });
    await expect(replicas).toHaveValue(String(starting), { timeout: 60_000 });
    await replicas.fill(String(original));
    await press(page, 'Scale');
    await expect
      .poll(() => field('deployment', 'healthy', '.spec.replicas'), { timeout: 60_000 })
      .toBe(String(original));
  } finally {
    scale('healthy', original);
  }
});

test('a rollout restart stamps the pod template the apiserver keeps', async ({ page }) => {
  const original = annotations('chatty');
  const stamp = (): string => field('deployment', 'chatty', '.spec.template.metadata.annotations');
  const before = stamp();
  try {
    await openWorkload(page, 'chatty');
    await press(page, 'Restart', true);
    await expect.poll(stamp, { timeout: 60_000 }).not.toBe(before);
    expect(stamp()).toContain('restartedAt');
  } finally {
    restoreAnnotations('chatty', original);
  }
});

test('cordoning a node takes it out of scheduling, and uncordoning puts it back', async ({
  page,
}) => {
  const node = kubectl([
    'get',
    'nodes',
    '-l',
    'spinoza.test/pool=drain',
    '-o',
    'jsonpath={.items[0].metadata.name}',
  ]).trim();
  expect(node).not.toBe('');
  const wasCordoned = field('node', node, '.spec.unschedulable', false) === 'true';
  if (wasCordoned) {
    kubectl(['uncordon', node]);
  }
  try {
    await openNode(page, node);
    await press(page, 'Cordon', true);
    await expect
      .poll(() => field('node', node, '.spec.unschedulable', false), { timeout: 60_000 })
      .toBe('true');
    await expect(page.getByRole('button', { name: 'Uncordon', exact: true })).toBeVisible({
      timeout: 30_000,
    });
    await press(page, 'Uncordon');
    await expect
      .poll(() => field('node', node, '.spec.unschedulable', false), { timeout: 60_000 })
      .toBe('');
  } finally {
    if (wasCordoned) {
      kubectl(['cordon', node]);
    } else {
      kubectl(['uncordon', node]);
    }
  }
});

test('a drain preview classifies every live pod without touching the node', async ({ page }) => {
  const node = kubectl([
    'get',
    'nodes',
    '-l',
    'spinoza.test/pool=drain',
    '-o',
    'jsonpath={.items[0].metadata.name}',
  ]).trim();
  expect(node).not.toBe('');
  const scheduled = field('node', node, '.spec.unschedulable', false);
  const before = podsOnNode(node);
  expect(before.length).toBeGreaterThan(0);
  try {
    await openNode(page, node);
    const planned = page.waitForResponse(
      (response) => {
        const url = new URL(response.url());
        return (
          url.pathname === '/api/action' &&
          url.searchParams.get('action') === 'drain' &&
          url.searchParams.get('dryRun') === 'true' &&
          response.request().method() === 'POST'
        );
      },
      { timeout: 60_000 },
    );
    await page.getByRole('button', { name: 'Drain', exact: true }).click();
    const response = await planned;
    expect(response.ok()).toBe(true);
    const result = (await response.json()) as ActionResult;
    expect(result).toMatchObject({ action: 'drain', dryRun: true });
    expect(result.pods).toBeDefined();
    if (result.pods === undefined) {
      throw new Error('the drain preview returned no pod classifications');
    }
    const outcomes = result.pods;
    const expected = before.map((pod) => `${pod.metadata.namespace}/${pod.metadata.name}`);
    const actual = outcomes
      .map((pod) => `${pod.namespace}/${pod.name}`)
      .sort((left, right) => left.localeCompare(right));
    expect(actual).toEqual(expected);

    const plan = page.getByText(result.message, { exact: true }).locator('..');
    await expect(plan).toBeVisible({ timeout: 30_000 });
    await expect(plan.getByRole('listitem')).toHaveCount(outcomes.length);
    for (const outcome of outcomes) {
      const row = plan.getByRole('listitem').filter({ hasText: outcome.name });
      await expect(row).toContainText(outcome.outcome);
      if (outcome.reason !== undefined) {
        await expect(row).toContainText(outcome.reason);
      }
    }
    await expect(plan.getByRole('button', { name: 'Drain now', exact: true })).toBeVisible();
    expect(field('node', node, '.spec.unschedulable', false)).toBe(scheduled);
    expect(podsOnNode(node).map((pod) => pod.metadata.uid)).toEqual(
      before.map((pod) => pod.metadata.uid),
    );
    await plan.getByRole('button', { name: 'Cancel', exact: true }).click();
    await expect(page.getByText(result.message, { exact: true })).toHaveCount(0);
  } finally {
    if (scheduled === 'true') {
      kubectl(['cordon', node]);
    } else {
      kubectl(['uncordon', node]);
    }
  }
});

test('suspending a cronjob reaches the apiserver, and resuming undoes it', async ({ page }) => {
  const original = JSON.parse(
    kubectl(['-n', NAMESPACE, 'get', 'cronjob/nightly', '-o', 'json']),
  ) as CronJob;
  kubectl([
    '-n',
    NAMESPACE,
    'patch',
    'cronjob/nightly',
    '--type=merge',
    '-p',
    '{"spec":{"suspend":false}}',
  ]);
  try {
    await openGrouped(page, 'batch', 'cronjobs', 'CronJob');
    await selectRow(page, 'nightly');
    await page.getByRole('tab', { name: 'Overview', exact: true }).click();
    await press(page, 'Suspend', true);
    await expect
      .poll(() => field('cronjob', 'nightly', '.spec.suspend'), { timeout: 60_000 })
      .toBe('true');
    await press(page, 'Resume');
    await expect
      .poll(() => field('cronjob', 'nightly', '.spec.suspend'), { timeout: 60_000 })
      .toBe('false');
  } finally {
    let suspend: boolean | null = null;
    if (original.spec.suspend !== undefined) {
      suspend = original.spec.suspend;
    }
    kubectl([
      '-n',
      NAMESPACE,
      'patch',
      'cronjob/nightly',
      '--type=merge',
      '-p',
      JSON.stringify({ spec: { suspend } }),
    ]);
  }
});

test('running a cronjob now creates the job it would have created on schedule', async ({
  page,
}) => {
  const before = new Set(nightlyJobs());
  try {
    await openGrouped(page, 'batch', 'cronjobs', 'CronJob');
    await selectRow(page, 'nightly');
    await page.getByRole('tab', { name: 'Overview', exact: true }).click();
    await press(page, 'Run now', true);
    await expect
      .poll(() => nightlyJobs().filter((name) => !before.has(name)).length, { timeout: 60_000 })
      .toBeGreaterThan(0);
  } finally {
    for (const name of nightlyJobs()) {
      if (before.has(name)) {
        continue;
      }
      kubectl(['-n', NAMESPACE, 'delete', `job/${name}`, '--ignore-not-found']);
    }
  }
});

test('a bulk restart names how many objects it is about to touch', async ({ page }) => {
  await openGrouped(page, 'apps', 'deployments', 'Deployment');
  await page.getByRole('checkbox', { name: 'Select healthy' }).check();
  await page.getByRole('checkbox', { name: 'Select chatty' }).check();
  const bar = page.locator('main').getByRole('status').filter({ hasText: 'selected' });
  await expect(bar).toContainText(/2\s*Deployment objects selected/, { timeout: 30_000 });
  for (const action of ['Restart', 'Delete', 'Clear selection']) {
    await expect(bar.getByRole('button', { name: action, exact: true })).toBeVisible();
  }
  await page.getByRole('button', { name: 'Clear selection', exact: true }).click();
  await expect(page.getByRole('checkbox', { name: 'Select healthy' })).not.toBeChecked();
});

test('an invalid replica count is refused before reaching the apiserver', async ({ page }) => {
  const before = field('deployment', 'healthy', '.spec.replicas');
  await openWorkload(page, 'healthy');
  await page.getByRole('spinbutton', { name: 'replicas' }).fill('-1');
  await page.getByRole('button', { name: 'Scale', exact: true }).click();
  await expect(page.getByText('replicas must be a whole number, zero or more')).toBeVisible();
  expect(field('deployment', 'healthy', '.spec.replicas')).toBe(before);
});

test('cancelling a restart leaves the pod template untouched', async ({ page }) => {
  const before = field('deployment', 'healthy', '.spec.template.metadata.annotations');
  await openWorkload(page, 'healthy');
  await page.getByRole('button', { name: 'Restart', exact: true }).click();
  await expect(page.getByText('Restart healthy? Every pod is replaced.')).toBeVisible();
  await page.getByRole('button', { name: 'Cancel', exact: true }).click();
  await expect(page.getByText('Restart healthy? Every pod is replaced.')).toHaveCount(0);
  expect(field('deployment', 'healthy', '.spec.template.metadata.annotations')).toBe(before);
});

test('cancelling scale to zero preserves every replica', async ({ page }) => {
  const before = field('deployment', 'healthy', '.spec.replicas');
  await openWorkload(page, 'healthy');
  await page.getByRole('spinbutton', { name: 'replicas' }).fill('0');
  await page.getByRole('button', { name: 'Scale', exact: true }).click();
  await expect(page.getByText('Scale healthy to zero? Every pod is removed.')).toBeVisible();
  await page.getByRole('button', { name: 'Cancel', exact: true }).click();
  expect(field('deployment', 'healthy', '.spec.replicas')).toBe(before);
});

test('bulk restart replaces both selected pod templates', async ({ page }) => {
  const healthyAnnotations = annotations('healthy');
  const chattyAnnotations = annotations('chatty');
  const healthyBefore = field('deployment', 'healthy', '.spec.template.metadata.annotations');
  const chattyBefore = field('deployment', 'chatty', '.spec.template.metadata.annotations');
  try {
    await openGrouped(page, 'apps', 'deployments', 'Deployment');
    await page.getByRole('checkbox', { name: 'Select healthy' }).check();
    await page.getByRole('checkbox', { name: 'Select chatty' }).check();
    const bar = page.locator('main').getByRole('status').filter({ hasText: 'selected' });
    await bar.getByRole('button', { name: 'Restart', exact: true }).click();
    await expect(bar).toContainText('Restart 2 objects?', { timeout: 30_000 });
    await bar.getByRole('button', { name: 'Confirm', exact: true }).click();
    await expect
      .poll(() => field('deployment', 'healthy', '.spec.template.metadata.annotations'), {
        timeout: 60_000,
      })
      .not.toBe(healthyBefore);
    await expect
      .poll(() => field('deployment', 'chatty', '.spec.template.metadata.annotations'), {
        timeout: 60_000,
      })
      .not.toBe(chattyBefore);
  } finally {
    restoreAnnotations('healthy', healthyAnnotations);
    restoreAnnotations('chatty', chattyAnnotations);
  }
});

test('bulk delete can be reviewed and cancelled without deleting anything', async ({ page }) => {
  await openGrouped(page, 'apps', 'deployments', 'Deployment');
  await page.getByRole('checkbox', { name: 'Select healthy' }).check();
  await page.getByRole('checkbox', { name: 'Select chatty' }).check();
  const bar = page.locator('main').getByRole('status').filter({ hasText: 'selected' });
  await bar.getByRole('button', { name: 'Delete', exact: true }).click();
  await expect(bar).toContainText('Delete 2 objects?', { timeout: 30_000 });
  await bar.getByRole('button', { name: 'Cancel', exact: true }).click();
  expect(field('deployment', 'healthy', '.metadata.name')).toBe('healthy');
  expect(field('deployment', 'chatty', '.metadata.name')).toBe('chatty');
});

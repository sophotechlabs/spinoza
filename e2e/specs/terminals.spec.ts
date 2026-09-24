import { expect, test } from '../harness/test';
import { openGrouped, openResource, selectRow } from '../harness/app';
import { kubectl, kubectlApply, kubectlSoft } from '../harness/cluster';
import type { Page } from '@playwright/test';

async function openLogs(page: Page, pod: string): Promise<void> {
  await openResource(page, 'pods', 'Pod');
  const row = page.locator('main tbody tr').filter({ hasText: pod }).first();
  await expect(row).toBeVisible({ timeout: 60_000 });
  await row.getByRole('button').first().click();
  await page.getByRole('tab', { name: 'Logs' }).click();
}

async function openWorkloadLogs(page: Page, workload: string): Promise<void> {
  await openGrouped(page, 'apps', 'deployments', 'Deployment');
  await selectRow(page, workload);
  await page.getByRole('tab', { name: 'Logs' }).click();
}

async function withControlledLogs(
  page: Page,
  name: string,
  run: () => Promise<void>,
): Promise<void> {
  try {
    kubectlApply(
      JSON.stringify({
        apiVersion: 'v1',
        kind: 'Pod',
        metadata: { name, namespace: 'e2e' },
        spec: {
          containers: [
            {
              name: 'writer',
              image: 'busybox:1.37',
              command: [
                'sh',
                '-c',
                'echo controlled-initial > /tmp/events\ntail -n +1 -f /tmp/events',
              ],
              readinessProbe: {
                exec: { command: ['test', '-f', '/tmp/events'] },
                periodSeconds: 1,
              },
            },
          ],
        },
      }),
    );
    kubectl(['wait', '--for=condition=Ready', `pod/${name}`, '-n', 'e2e', '--timeout=90s']);
    await openLogs(page, name);
    await expect(page.getByText('controlled-initial', { exact: true })).toBeVisible({
      timeout: 60_000,
    });
    await run();
  } finally {
    kubectlSoft(['delete', 'pod', name, '-n', 'e2e', '--ignore-not-found', '--wait=false']);
  }
}

function emitLogs(name: string, lines: string[]): void {
  kubectl([
    'exec',
    '-n',
    'e2e',
    name,
    '--',
    'sh',
    '-c',
    'printf "%s\\n" "$@" >> /tmp/events',
    'writer',
    ...lines,
  ]);
}

test('the log panel offers the controls it promises', async ({ page }) => {
  await openLogs(page, 'chatty');
  for (const control of [
    'Following',
    'Pretty',
    'Timestamps',
    'Wrap',
    'Copy',
    'Download',
    'Clear',
  ]) {
    await expect(page.getByRole('button', { name: control, exact: true })).toBeVisible({
      timeout: 30_000,
    });
  }
  await expect(page.getByLabel('Filter log lines')).toBeVisible();
});

test('a container that prints reaches the log panel', async ({ page }) => {
  await openLogs(page, 'chatty');
  await expect(page.getByText('e2e-log-line').first()).toBeVisible({ timeout: 60_000 });
});

test('pausing follow stops the scroll, not the stream', async ({ page }) => {
  const name = 'e2e-log-pause';
  await withControlledLogs(page, name, async () => {
    await page.getByLabel('Filter log lines').fill('controlled-');
    emitLogs(
      name,
      Array.from({ length: 200 }, (_, index) => `controlled-before-${String(index)}`),
    );
    await expect(page.getByText('201 of 201', { exact: true })).toBeVisible({ timeout: 30_000 });
    const scroll = page
      .getByRole('tabpanel', { name: 'Logs' })
      .locator('[data-index]')
      .first()
      .locator('../..');
    await expect.poll(() => scroll.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
    await page.getByRole('button', { name: 'Following', exact: true }).click();
    await scroll.evaluate((element) => {
      element.scrollTop = 0;
    });
    emitLogs(
      name,
      Array.from({ length: 50 }, (_, index) => `controlled-after-${String(index)}`),
    );
    await expect(page.getByText('251 of 251', { exact: true })).toBeVisible({ timeout: 30_000 });
    expect(await scroll.evaluate((element) => element.scrollTop)).toBe(0);
    await expect(page.getByRole('button', { name: 'Follow', exact: true })).toHaveAttribute(
      'aria-pressed',
      'false',
    );
    await page.getByRole('button', { name: 'Follow', exact: true }).click();
    await expect(page.getByText('controlled-after-49', { exact: true })).toBeVisible();
    await expect.poll(() => scroll.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
  });
});

test('the filter narrows the lines that are shown', async ({ page }) => {
  await openLogs(page, 'chatty');
  await expect(page.getByText('e2e-log-line').first()).toBeVisible({ timeout: 60_000 });
  await page.getByLabel('Filter log lines').fill('nothing-matches-this');
  await expect(page.getByText('e2e-log-line')).toHaveCount(0, { timeout: 30_000 });
});

test('clearing empties the panel without stopping the stream', async ({ page }) => {
  const name = 'e2e-log-clear';
  await withControlledLogs(page, name, async () => {
    await page.getByRole('button', { name: 'Clear', exact: true }).click();
    await expect(page.getByText('controlled-initial', { exact: true })).toHaveCount(0);
    await expect(page.getByText('Waiting for output', { exact: true })).toBeVisible();
    emitLogs(name, ['controlled-after-clear']);
    await expect(page.getByText('controlled-after-clear', { exact: true })).toBeVisible({
      timeout: 30_000,
    });
    await expect(page.getByText('controlled-initial', { exact: true })).toHaveCount(0);
  });
});

test('pretty and raw presentation can be toggled independently', async ({ page }) => {
  await openLogs(page, 'chatty');
  const pretty = page.getByRole('button', { name: 'Pretty', exact: true });
  await expect(pretty).toHaveAttribute('aria-pressed', 'true');
  await pretty.click();
  const raw = page.getByRole('button', { name: 'Raw', exact: true });
  await expect(raw).toHaveAttribute('aria-pressed', 'false');
  await raw.click();
  await expect(page.getByRole('button', { name: 'Pretty', exact: true })).toHaveAttribute(
    'aria-pressed',
    'true',
  );
});

test('timestamp and wrapping controls expose and restore their state', async ({ page }) => {
  await openLogs(page, 'chatty');
  const timestamps = page.getByRole('button', { name: 'Timestamps', exact: true });
  const wrap = page.getByRole('button', { name: 'Wrap', exact: true });
  await expect(timestamps).toHaveAttribute('aria-pressed', 'true');
  await expect(wrap).toHaveAttribute('aria-pressed', 'true');
  await timestamps.click();
  await wrap.click();
  await expect(timestamps).toHaveAttribute('aria-pressed', 'false');
  await expect(wrap).toHaveAttribute('aria-pressed', 'false');
  await timestamps.click();
  await wrap.click();
  await expect(timestamps).toHaveAttribute('aria-pressed', 'true');
  await expect(wrap).toHaveAttribute('aria-pressed', 'true');
});

test('pausing offers an explicit jump without silently resuming', async ({ page }) => {
  await openLogs(page, 'chatty');
  await expect(page.getByText('e2e-log-line').first()).toBeVisible({ timeout: 60_000 });
  await page.getByRole('button', { name: 'Following', exact: true }).click();
  const jump = page.getByRole('button', { name: 'Jump to bottom', exact: true });
  await expect(jump).toBeVisible();
  await jump.click();
  await expect(page.getByRole('button', { name: 'Follow', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Follow', exact: true }).click();
  await expect(jump).toHaveCount(0);
});

test('an empty log filter reports both the empty result and its scope', async ({ page }) => {
  await openLogs(page, 'chatty');
  await expect(page.getByText('e2e-log-line').first()).toBeVisible({ timeout: 60_000 });
  await page.getByLabel('Filter log lines').fill('nothing-matches-this');
  await expect(page.getByText('No line matches that filter.', { exact: true })).toBeVisible();
  await expect(page.getByText(/0 of \d+/)).toBeVisible();
});

test('download saves the visible log buffer under the pod and container name', async ({ page }) => {
  await openLogs(page, 'chatty');
  await expect(page.getByText('e2e-log-line').first()).toBeVisible({ timeout: 60_000 });
  const started = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Download', exact: true }).click();
  const download = await started;
  expect(download.suggestedFilename()).toMatch(/^e2e-chatty-.*-talker\.log$/);
  const stream = await download.createReadStream();
  expect(stream).not.toBeNull();
  let body = '';
  if (stream !== null) {
    for await (const chunk of stream) {
      body += chunk.toString();
    }
  }
  expect(body).toContain('e2e-log-line');
});

test('workload logs merge output from every matching pod', async ({ page }) => {
  const workload = 'e2e-log-merge';
  try {
    kubectlApply(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${workload}
  namespace: e2e
spec:
  replicas: 2
  selector:
    matchLabels:
      app: ${workload}
  template:
    metadata:
      labels:
        app: ${workload}
    spec:
      containers:
        - name: writer
          image: busybox:1.37
          command:
            - sh
            - -c
            - while true; do echo "$HOSTNAME e2e-workload-log"; sleep 1; done
`);
    kubectl(['--namespace', 'e2e', 'rollout', 'status', `deployment/${workload}`, '--timeout=90s']);
    const pods = kubectl([
      '--namespace',
      'e2e',
      'get',
      'pods',
      '--selector',
      `app=${workload}`,
      '--output',
      'name',
    ])
      .trim()
      .split('\n')
      .map((name) => name.replace('pod/', ''));
    expect(pods).toHaveLength(2);

    await openWorkloadLogs(page, workload);
    await expect(page.getByText('2 pods', { exact: true })).toBeVisible({ timeout: 60_000 });
    for (const pod of pods) {
      await expect(page.getByText(`${pod} e2e-workload-log`, { exact: true }).first()).toBeVisible({
        timeout: 60_000,
      });
    }
    await expect(page.getByText('e2e-workload-log').first()).toBeVisible({ timeout: 60_000 });
  } finally {
    kubectl([
      '--namespace',
      'e2e',
      'delete',
      'deployment',
      workload,
      '--ignore-not-found=true',
      '--wait=true',
    ]);
  }
});

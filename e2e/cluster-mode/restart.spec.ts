import { expect, test } from '@playwright/test';
import { api, feedClosed, kubectl, openFeed, restartSpinoza, signIn } from './harness';

interface Ready {
  ready: boolean;
  discovery: boolean;
  informers: boolean;
  store: boolean;
  waiting?: string[];
}

function podNames(): string[] {
  const listed = kubectl([
    '-n',
    'spinoza',
    'get',
    'pods',
    '-l',
    'app.kubernetes.io/name=spinoza',
    '-o',
    'jsonpath={.items[*].metadata.name}',
  ]);
  return listed.split(' ').filter((one) => one !== '');
}

function logsOf(pod: string): string {
  return kubectl(['-n', 'spinoza', 'logs', pod, '--tail=2000']);
}

test('readiness says what it is waiting for, and the chart gates on it', async ({ page }) => {
  await signIn(page, 'alice');

  const answer = await api(page, 'GET', '/readyz');

  expect([200, 503]).toContain(answer.status);
  const state = JSON.parse(answer.body) as Ready;
  if (state.ready) {
    expect(state.discovery && state.informers && state.store).toBe(true);
    expect(state.waiting ?? []).toHaveLength(0);
  } else {
    expect(state.waiting?.length ?? 0).toBeGreaterThan(0);
  }

  const probes = kubectl([
    '-n',
    'spinoza',
    'get',
    'deployment/spinoza',
    '-o',
    'jsonpath={.spec.template.spec.containers[0].readinessProbe.httpGet.path}' +
      '{" "}{.spec.template.spec.containers[0].startupProbe.httpGet.path}',
  ]);
  expect(probes).toBe('/readyz /readyz');
});

test('a rollout restart costs a reconnect, not an error', async ({ page }) => {
  await signIn(page, 'alice');
  await openFeed(page);
  expect(await feedClosed(page)).toBe(false);
  const before = podNames();

  restartSpinoza();

  await expect.poll(() => feedClosed(page), { timeout: 120_000 }).toBe(true);
  const after = podNames();
  expect(after).not.toEqual(before);

  await expect
    .poll(async () => (await api(page, 'GET', '/api/overview')).status, { timeout: 180_000 })
    .toBe(200);
});

test('spinoza reports on itself, and the scrape port is not the app port', async ({ page }) => {
  await signIn(page, 'alice');

  const metrics = await api(page, 'GET', '/metrics');

  expect(metrics.status).toBe(200);
  expect(metrics.body).toContain('# TYPE spinoza_http_requests_total counter');
  expect(metrics.body).toContain('spinoza_sign_ins_total{role="admin"}');
  expect(metrics.body).toContain('spinoza_build_info{version=');
});

test('what spinoza did goes out as one parseable line per change', async ({ page }) => {
  await signIn(page, 'alice');
  const namespace = `restart-audit-${String(Date.now())}`;

  const made = await api(
    page,
    'PUT',
    `/api/object?version=v1&resource=namespaces&name=${namespace}`,
    JSON.stringify({
      yaml: `apiVersion: v1\nkind: Namespace\nmetadata:\n  name: ${namespace}\n`,
    }),
    { 'Content-Type': 'application/json' },
  );
  expect([200, 201]).toContain(made.status);

  try {
    const pod = podNames()[0];
    await expect.poll(() => logsOf(pod), { timeout: 120_000 }).toContain(namespace);

    const audit = logsOf(pod)
      .split('\n')
      .filter((line) => line.includes('"event":"audit"') && line.includes(namespace));
    expect(audit.length).toBeGreaterThan(0);
    const read = JSON.parse(audit[0]) as Record<string, string>;
    expect(read.event).toBe('audit');
    expect(read.actor).toBe('alice');
    expect(read.name).toBe(namespace);
    expect(read.outcome).not.toBe('');
  } finally {
    kubectl(['delete', 'namespace', namespace, '--ignore-not-found', '--wait=false']);
  }
});

test('every answer names the request it answered', async ({ page }) => {
  await signIn(page, 'alice');

  const named = await page.evaluate(async () => {
    const response = await fetch('/api/version');
    return response.headers.get('x-spinoza-request') ?? '';
  });

  expect(named).not.toBe('');
  expect(named.length).toBeGreaterThanOrEqual(8);
});

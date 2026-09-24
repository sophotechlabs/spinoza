import { createServer } from 'node:http';
import { rmSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test } from '../harness/test';
import { kubectlApply, kubectlSoft } from '../harness/cluster';
import { CONTEXT, KUBECONFIG, TMP_DIR, sideAddr } from '../harness/paths';
import { freePort, launch, stop } from '../harness/spinoza';
import type { Instance } from '../harness/spinoza';
import { hold } from '../harness/keepalive';
import type { Held } from '../harness/keepalive';

interface AuditRun {
  id: string;
  findings: number;
  scanned: number;
  new?: number;
  cleared?: number;
}

test('scheduled audits persist runs, retry a refused webhook, suppress unchanged notices, and report a new finding', async ({
  page,
}) => {
  test.setTimeout(300_000);
  const notices: { body: string; type: string | undefined }[] = [];
  const sink = createServer((request, response) => {
    const chunks: Buffer[] = [];
    request.on('data', (chunk: Buffer) => chunks.push(chunk));
    request.on('end', () => {
      notices.push({
        body: Buffer.concat(chunks).toString('utf8'),
        type: request.headers['content-type'],
      });
      response.statusCode = 204;
      if (notices.length === 1) {
        response.statusCode = 503;
      }
      response.end();
    });
  });
  await new Promise<void>((resolve) => sink.listen(0, '127.0.0.1', resolve));
  const address = sink.address();
  if (address === null || typeof address === 'string') {
    throw new Error('the audit webhook sink has no TCP port');
  }
  let instance: Instance | undefined;
  let held: Held | undefined;
  const home = join(TMP_DIR, 'home-audit-schedule');
  const namespace = 'e2e-audit-schedule';
  const name = 'e2e-audit-marker';
  try {
    kubectlApply(
      JSON.stringify({ apiVersion: 'v1', kind: 'Namespace', metadata: { name: namespace } }),
    );
    kubectlApply(
      JSON.stringify({
        apiVersion: 'v1',
        kind: 'ConfigMap',
        metadata: { name, namespace },
        data: { state: 'unchanged' },
      }),
    );
    await freePort(sideAddr(8).split(':')[1]);
    rmSync(home, { recursive: true, force: true });
    instance = await launch({
      name: 'audit-schedule',
      addr: sideAddr(8),
      kubeconfig: KUBECONFIG,
      tokenFile: join(TMP_DIR, 'token-audit-schedule'),
      home,
      extra: [
        '--audit-interval=1m',
        `--audit-webhook=http://127.0.0.1:${String(address.port)}/notice`,
      ],
    });
    held = hold(instance.baseURL, instance.token);
    const baseURL = instance.baseURL;
    await page.goto(
      `${baseURL}/?token=${encodeURIComponent(instance.token)}#context=${CONTEXT}&view=checks`,
    );
    const checks = await page.request.get(`${baseURL}/api/checks?everyKind=1`);
    expect(checks.status()).toBe(200);
    const builtins = (await checks.json()) as { groups: { id: string }[] };
    expect(builtins.groups.length).toBeGreaterThan(0);
    for (const group of builtins.groups) {
      const muted = await page.request.post(`${baseURL}/api/checks/mutes`, {
        data: {
          check: group.id,
          reason: 'This isolated scheduler fixture uses its own fixed findings.',
        },
      });
      expect(muted.status()).toBe(200);
    }
    const match = `object.metadata.namespace == '${namespace}' && object.metadata.name == '${name}'`;
    const rule = {
      id: 'e2e-audit-state',
      match: 'ConfigMap',
      expr: `${match} && object.data.state == 'changed'`,
      severity: 'high',
    };
    const fixed = { id: 'e2e-audit-fixed', match: 'ConfigMap', expr: match, severity: 'low' };
    const saved = await page.request.put(`${baseURL}/api/settings`, {
      data: { values: { 'spinoza.checks.rules.v1': JSON.stringify([fixed, rule]) } },
    });
    expect(saved.status()).toBe(200);
    async function runs(): Promise<AuditRun[]> {
      const response = await page.request.get(`${baseURL}/api/checks/runs`);
      expect(response.status()).toBe(200);
      const body = (await response.json()) as { runs: AuditRun[]; interval: number };
      expect(body.interval).toBe(60);
      return body.runs;
    }
    await expect
      .poll(async () => (await runs()).length, { timeout: 100_000, intervals: [1000] })
      .toBeGreaterThanOrEqual(1);
    const first = (await runs())[0];
    expect(first.findings).toBe(1);
    await expect.poll(() => notices.length, { timeout: 15_000 }).toBe(2);
    expect(notices[0].type).toBe('application/json');
    expect(notices[1]).toEqual(notices[0]);
    expect(JSON.parse(notices[0].body)).toMatchObject({
      findings: first.findings,
      scanned: first.scanned,
    });
    await expect
      .poll(async () => (await runs()).length, { timeout: 90_000, intervals: [1000] })
      .toBeGreaterThanOrEqual(2);
    const second = (await runs())[0];
    expect(second).toMatchObject({
      findings: first.findings,
      scanned: first.scanned,
    });
    expect(second.new).toBe(first.new);
    expect(second.cleared).toBe(first.cleared);
    expect(second.id).not.toBe(first.id);
    expect(notices).toHaveLength(2);
    kubectlApply(
      JSON.stringify({
        apiVersion: 'v1',
        kind: 'ConfigMap',
        metadata: { name, namespace },
        data: { state: 'changed' },
      }),
    );
    await expect.poll(() => notices.length, { timeout: 90_000, intervals: [1000] }).toBe(3);
    const third = (await runs())[0];
    expect(third.findings).toBe(first.findings + 1);
    expect(third.scanned).toBe(first.scanned);
    expect(JSON.parse(notices[2].body)).toMatchObject({
      findings: third.findings,
      scanned: third.scanned,
    });
    await page.reload();
    await expect(page.getByRole('button', { name: /e2e-audit-state/ })).toBeVisible({
      timeout: 30_000,
    });
  } finally {
    if (held !== undefined) {
      await held.close();
    }
    if (instance !== undefined) {
      await stop('audit-schedule', instance.pid);
    }
    await new Promise<void>((resolve, reject) =>
      sink.close((error) => {
        if (error !== undefined) {
          reject(error);
          return;
        }
        resolve();
      }),
    );
    kubectlSoft(['delete', 'namespace', namespace, '--ignore-not-found', '--wait=false']);
  }
});

import { readFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import type { Page } from '@playwright/test';
import { expect, test } from '../harness/test';
import { openView, selectRow } from '../harness/app';
import { CONTEXT, KUBECONFIG, TMP_DIR, sideAddr } from '../harness/paths';
import { freePort, launch, stop } from '../harness/spinoza';
import type { Instance } from '../harness/spinoza';
import { hold } from '../harness/keepalive';
import type { Held } from '../harness/keepalive';

const home = join(TMP_DIR, 'home-recordings');
const tokenFile = join(TMP_DIR, 'token-recordings');
let instance: Instance;
let held: Held;

async function startRecording(): Promise<void> {
  instance = await launch({
    name: 'recordings',
    addr: sideAddr(7),
    kubeconfig: KUBECONFIG,
    tokenFile,
    home,
    extra: ['--record-sessions'],
  });
  held = hold(instance.baseURL, instance.token);
}

test.beforeAll(async () => {
  await freePort(sideAddr(7).split(':')[1]);
  rmSync(home, { recursive: true, force: true });
  await startRecording();
});

test.afterAll(async () => {
  await held.close();
  await stop('recordings', instance.pid);
});

async function openRecordingView(page: Page, hash: string): Promise<void> {
  await page.goto(
    `${instance.baseURL}/?token=${encodeURIComponent(instance.token)}#context=${CONTEXT}&${hash}`,
  );
}

async function openRecordings(page: Page): Promise<void> {
  await openRecordingView(page, 'view=history');
  await page.getByRole('button', { name: 'Recorded sessions', exact: true }).click();
}

test('recorded terminal output survives a server restart and remains readable in history', async ({
  page,
}) => {
  test.setTimeout(150_000);
  await openRecordingView(page, 'version=v1&resource=pods&kind=Pod');
  await selectRow(page, 'shellable');
  await page.getByRole('tab', { name: 'Terminal', exact: true }).click();
  await page.getByRole('button', { name: 'Shell in shellable' }).click();
  const terminal = page.locator('.xterm-screen').first();
  await expect(terminal).toBeVisible({ timeout: 60_000 });
  await terminal.click();
  await page.keyboard.type("printf 'recorded-%s\\n' 'output-7f93'");
  await page.keyboard.press('Enter');
  await expect(page.locator('.xterm-rows').first()).toContainText('recorded-output-7f93');
  await page.keyboard.type('exit');
  await page.keyboard.press('Enter');
  await openRecordings(page);
  const session = page.locator('main li').filter({ hasText: 'shellable' }).first();
  await expect(session).toContainText('exec');
  await session.getByRole('button', { name: 'Read', exact: true }).click();
  await expect(session.locator('pre')).toContainText('recorded-output-7f93');
  const response = await page.request.get(`${instance.baseURL}/api/transcripts`);
  expect(response.status()).toBe(200);
  const before = (await response.json()) as {
    recording: boolean;
    sessions: { id: string; bytes: number }[];
  };
  expect(before.recording).toBe(true);
  expect(before.sessions).toHaveLength(1);
  expect(before.sessions[0].bytes).toBeGreaterThan(0);
  const id = before.sessions[0].id;
  await held.close();
  await stop('recordings', instance.pid);
  await startRecording();
  await openRecordings(page);
  await session.getByRole('button', { name: 'Read', exact: true }).click();
  await expect(session.locator('pre')).toContainText('recorded-output-7f93');
  const text = await page.request.get(
    `${instance.baseURL}/api/transcripts/text?id=${encodeURIComponent(id)}`,
  );
  expect(text.status()).toBe(200);
  expect(text.headers()['content-type']).toBe('text/plain; charset=utf-8');
  expect(text.headers()['x-content-type-options']).toBe('nosniff');
  expect(text.headers()['content-disposition']).toBe('attachment; filename="spinoza-session.txt"');
  expect(await text.text()).toContain('recorded-output-7f93');
  await session.getByRole('button', { name: 'Hide', exact: true }).click();
  await expect(session.locator('pre')).toHaveCount(0);
});

test('unknown and path-shaped recording IDs cannot read files from the server', async ({
  page,
}) => {
  await openRecordings(page);
  for (const id of ['unknown-session', '../../token-recordings', tokenFile]) {
    const response = await page.request.get(
      `${instance.baseURL}/api/transcripts/text?id=${encodeURIComponent(id)}`,
    );
    expect(response.status()).toBe(404);
    expect(await response.text()).not.toContain(instance.token);
  }
  const sessions = await page.request.get(`${instance.baseURL}/api/transcripts`);
  expect(sessions.status()).toBe(200);
  expect(((await sessions.json()) as { recording: boolean }).recording).toBe(true);
});

test('a deployment with recording disabled reports the reason and refuses transcript reads', async ({
  page,
}) => {
  await openView(page, 'history');
  await page.getByRole('button', { name: 'Recorded sessions', exact: true }).click();
  const reason = 'this deployment does not record terminal sessions';
  await expect(page.getByText(reason, { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Read', exact: true })).toHaveCount(0);
  const listed = await page.request.get('/api/transcripts');
  expect(listed.status()).toBe(200);
  expect(await listed.json()).toEqual({ recording: false, sessions: [], reason });
  const read = await page.request.get('/api/transcripts/text?id=unknown-session');
  expect(read.status()).toBe(404);
  expect(await read.json()).toEqual({ message: reason, request: expect.stringMatching(/\S+/) });
});

test('support bundles download useful diagnostics while excluding setting values and credentials', async ({
  page,
}, testInfo) => {
  await openRecordingView(page, 'view=history');
  const sensitive = 'support-fixture-password-8c4f';
  const stored = await page.request.put(`${instance.baseURL}/api/settings`, {
    data: { values: { 'e2e.support-marker': sensitive } },
  });
  expect(stored.status()).toBe(200);
  await page.getByRole('button', { name: 'Settings', exact: true }).click();
  await page
    .getByRole('navigation', { name: 'Settings sections' })
    .getByRole('button', { name: 'About', exact: true })
    .click();
  const downloading = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Save a support bundle', exact: true }).click();
  const download = await downloading;
  expect(download.suggestedFilename()).toBe('spinoza-support.json');
  const destination = testInfo.outputPath('support.json');
  await download.saveAs(destination);
  const raw = readFileSync(destination, 'utf8');
  const bundle = JSON.parse(raw) as {
    taken: string;
    version: string;
    recording: { sessions: boolean };
    settingKeys: Record<string, string>;
    clusters: { name: string; reachable: boolean; kinds: number }[];
    whatThisDoesNotCarry: string[];
    metrics: string;
  };
  expect(bundle.settingKeys['e2e.support-marker']).toBe('set, under a kilobyte');
  expect(bundle.recording.sessions).toBe(true);
  expect(bundle.version).not.toBe('');
  expect(Number.isNaN(Date.parse(bundle.taken))).toBe(false);
  expect(bundle.clusters).toEqual(
    expect.arrayContaining([expect.objectContaining({ name: CONTEXT, reachable: true })]),
  );
  expect(bundle.clusters.find((cluster) => cluster.name === CONTEXT)?.kinds).toBeGreaterThan(0);
  expect(bundle.metrics).toContain('spinoza_');
  expect(bundle.whatThisDoesNotCarry).toContain(
    'no cluster object, log line or terminal transcript',
  );
  for (const excluded of [
    sensitive,
    instance.token,
    'recorded-output-7f93',
    'certificate-authority-data',
  ]) {
    expect(raw).not.toContain(excluded);
  }
});

test('diagnostics and recordings require the run token before exposing any data', async ({
  page,
  playwright,
}) => {
  await openRecordings(page);
  const anonymous = await playwright.request.newContext();
  try {
    for (const path of [
      '/api/support',
      '/api/history/export?format=json',
      '/api/transcripts',
      '/api/transcripts/text?id=unknown',
    ]) {
      const refused = await anonymous.get(`${instance.baseURL}${path}`);
      expect(refused.status()).toBe(401);
      expect(await refused.json()).toEqual({
        message: 'spinoza needs the token it printed at startup',
        request: expect.stringMatching(/\S+/),
      });
    }
    const admitted = await page.request.get(`${instance.baseURL}/api/support`);
    expect(admitted.status()).toBe(200);
    expect(
      ((await admitted.json()) as { recording: { sessions: boolean } }).recording.sessions,
    ).toBe(true);
  } finally {
    await anonymous.dispose();
  }
});

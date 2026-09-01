import { request } from '@playwright/test';
import type { Browser, Page } from '@playwright/test';
import { expect, holdSide, side, sideAuthed, test } from '../harness/test';
import type { Held } from '../harness/keepalive';
import { kubectl } from '../harness/cluster';
import { CONTEXT, NAMESPACE } from '../harness/paths';

const GUARDED = 'guarded-by-e2e';

let release: Held;

async function protect(on: boolean): Promise<void> {
  const one = side('guarded');
  const client = await request.newContext({
    extraHTTPHeaders: { 'X-Spinoza-Token': one.token },
  });
  const response = await client.post(`${one.baseURL}/api/protection?protected=${String(on)}`);
  expect(response.status()).toBe(200);
  await client.dispose();
}

async function openGuardedYaml(browser: Browser): Promise<[Page, () => Promise<void>]> {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(
    sideAuthed('guarded', `#context=${CONTEXT}&version=v1&resource=configmaps&kind=ConfigMap`),
  );
  await page.waitForLoadState('domcontentloaded');
  const row = page.locator('main tbody tr').filter({ hasText: GUARDED }).first();
  await row.waitFor({ state: 'visible', timeout: 90_000 });
  await row.click();
  await page.getByRole('tab', { name: 'YAML', exact: true }).click();
  await expect(page.locator('.monaco-editor').first()).toBeVisible({ timeout: 60_000 });
  return [page, async () => context.close()];
}

function documentFor(version: string, value: string): string {
  return JSON.stringify({
    apiVersion: 'v1',
    kind: 'ConfigMap',
    metadata: { name: GUARDED, namespace: NAMESPACE, resourceVersion: version },
    data: { gate: value },
  });
}

function liveVersion(): string {
  return kubectl([
    '-n',
    NAMESPACE,
    'get',
    `configmap/${GUARDED}`,
    '-o',
    'jsonpath={.metadata.resourceVersion}',
  ]).trim();
}

function gateValue(): string {
  return kubectl([
    '-n',
    NAMESPACE,
    'get',
    `configmap/${GUARDED}`,
    '-o',
    'jsonpath={.data.gate}',
  ]).trim();
}

async function editorText(page: Page): Promise<string> {
  const raw = await page.locator('.view-lines').first().innerText();
  return raw.replace(/\u00a0/g, ' ');
}

async function typeInto(page: Page, text: string): Promise<void> {
  await page.locator('.view-lines').first().click({ position: { x: 5, y: 5 } });
  await page.getByRole('textbox', { name: 'Editor content' }).focus();
  await page.keyboard.press('ControlOrMeta+a');
  await page.keyboard.press('Delete');
  await page.keyboard.insertText(text);
  await expect
    .poll(async () => (await editorText(page)).trim(), { timeout: 20_000 })
    .toBe(text.trim());
}

test.beforeAll(async () => {
  release = holdSide('guarded');
  kubectl(['-n', NAMESPACE, 'delete', 'configmap', GUARDED, '--ignore-not-found']);
  kubectl(['-n', NAMESPACE, 'create', 'configmap', GUARDED, '--from-literal=gate=before']);
  await protect(true);
});

test.afterAll(async () => {
  await protect(false);
  kubectl(['-n', NAMESPACE, 'delete', 'configmap', GUARDED, '--ignore-not-found']);
  await release.close();
});

test('applying on a protected cluster asks for the name before it writes', async ({ browser }) => {
  const [page, close] = await openGuardedYaml(browser);

  await typeInto(page, documentFor(liveVersion(), 'asked'));
  await page.getByRole('button', { name: 'Apply', exact: true }).click();

  await expect(
    page.getByRole('dialog', { name: 'Confirm on a protected cluster' }),
  ).toBeVisible({ timeout: 30_000 });
  await expect(page.getByText(`Applying your changes to ConfigMap ${GUARDED}`)).toBeVisible();
  expect(gateValue()).toBe('before');
  await close();
});

test('the wrong name does not unlock the apply', async ({ browser }) => {
  const [page, close] = await openGuardedYaml(browser);
  await typeInto(page, documentFor(liveVersion(), 'wrong-name'));
  await page.getByRole('button', { name: 'Apply', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Confirm on a protected cluster' });
  await expect(dialog).toBeVisible({ timeout: 30_000 });

  await dialog.getByLabel('Name').fill('not-the-name');

  await expect(dialog.getByRole('button', { name: 'Confirm' })).toBeDisabled();
  expect(gateValue()).toBe('before');
  await close();
});

test('the name typed out lets the edit through to the apiserver', async ({ browser }) => {
  const [page, close] = await openGuardedYaml(browser);
  await typeInto(page, documentFor(liveVersion(), 'after'));
  await page.getByRole('button', { name: 'Apply', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Confirm on a protected cluster' });
  await expect(dialog).toBeVisible({ timeout: 30_000 });

  await dialog.getByLabel('Name').fill(GUARDED);
  await dialog.getByRole('button', { name: 'Confirm' }).click();

  await expect.poll(() => gateValue(), { timeout: 60_000 }).toBe('after');
  await close();
});

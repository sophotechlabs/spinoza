import { request } from '@playwright/test';
import type { Browser, Page } from '@playwright/test';
import { expect, holdSide, side, sideAuthed, test } from '../harness/test';
import type { Held } from '../harness/keepalive';
import { kubectl, kubectlSoft } from '../harness/cluster';
import { CONTEXT, NAMESPACE } from '../harness/paths';
import { replaceEditor } from '../harness/editor';

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
  await row.getByRole('button', { name: GUARDED, exact: true }).click();
  await page.getByRole('tab', { name: 'YAML', exact: true }).click();
  await expect(page.locator('.monaco-editor').first()).toBeVisible({ timeout: 60_000 });
  return [page, async () => context.close()];
}

async function openGuardedWorkload(browser: Browser): Promise<[Page, () => Promise<void>]> {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(
    sideAuthed(
      'guarded',
      `#context=${CONTEXT}&group=apps&version=v1&resource=deployments&kind=Deployment`,
    ),
  );
  await page.waitForLoadState('domcontentloaded');
  const row = page.locator('main tbody tr').filter({ hasText: 'healthy' }).first();
  await row.waitFor({ state: 'visible', timeout: 90_000 });
  await row.getByRole('button', { name: 'healthy', exact: true }).click();
  await page
    .getByRole('tablist', { name: 'right panels' })
    .waitFor({ state: 'visible', timeout: 60_000 });
  await page.getByRole('tab', { name: 'Overview', exact: true }).click();
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

test.beforeAll(async () => {
  release = holdSide('guarded');
  await protect(true);
});

test.beforeEach(() => {
  kubectl(['-n', NAMESPACE, 'delete', 'configmap', GUARDED, '--ignore-not-found']);
  kubectl(['-n', NAMESPACE, 'create', 'configmap', GUARDED, '--from-literal=gate=before']);
});

test.afterAll(async () => {
  await protect(false);
  kubectl(['-n', NAMESPACE, 'delete', 'configmap', GUARDED, '--ignore-not-found']);
  await release.close();
});

test('applying on a protected cluster asks for the name before it writes', async ({ browser }) => {
  const [page, close] = await openGuardedYaml(browser);

  await replaceEditor(page, documentFor(liveVersion(), 'asked'));
  await page.getByRole('button', { name: 'Apply', exact: true }).click();

  await expect(page.getByRole('dialog', { name: 'Confirm on a protected cluster' })).toBeVisible({
    timeout: 30_000,
  });
  await expect(page.getByText(`Applying your changes to ConfigMap ${GUARDED}`)).toBeVisible();
  expect(gateValue()).toBe('before');
  await close();
});

test('the wrong name does not unlock the apply', async ({ browser }) => {
  const [page, close] = await openGuardedYaml(browser);
  await replaceEditor(page, documentFor(liveVersion(), 'wrong-name'));
  await page.getByRole('button', { name: 'Apply', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Confirm on a protected cluster' });
  await expect(dialog).toBeVisible({ timeout: 30_000 });

  await dialog.getByRole('textbox', { name: 'Name', exact: true }).fill('not-the-name');

  await expect(dialog.getByRole('button', { name: 'Confirm' })).toBeDisabled();
  expect(gateValue()).toBe('before');
  await close();
});

test('the name typed out lets the edit through to the apiserver', async ({ browser }) => {
  const [page, close] = await openGuardedYaml(browser);
  await replaceEditor(page, documentFor(liveVersion(), 'after'));
  await page.getByRole('button', { name: 'Apply', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Confirm on a protected cluster' });
  await expect(dialog).toBeVisible({ timeout: 30_000 });

  await dialog.getByRole('textbox', { name: 'Name', exact: true }).fill(GUARDED);
  await dialog.getByRole('button', { name: 'Confirm' }).click();

  await expect.poll(() => gateValue(), { timeout: 60_000 }).toBe('after');
  await close();
});

test('scaling a protected workload to zero asks for its name and cancel leaves it running', async ({
  browser,
}) => {
  const original = Number(
    kubectl([
      '-n',
      NAMESPACE,
      'get',
      'deployment/healthy',
      '-o',
      'jsonpath={.spec.replicas}',
    ]).trim(),
  );
  expect(Number.isInteger(original)).toBe(true);
  let starting = original;
  if (starting <= 0) {
    starting = 2;
  }
  kubectl(['-n', NAMESPACE, 'scale', 'deployment/healthy', `--replicas=${String(starting)}`]);
  const [page, close] = await openGuardedWorkload(browser);
  try {
    await page.getByRole('spinbutton', { name: 'replicas' }).fill('0');
    await page.getByRole('button', { name: 'Scale', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: 'Confirm on a protected cluster' });
    await expect(dialog).toContainText('Scale healthy to zero? Every pod is removed.');
    expect(
      kubectl([
        '-n',
        NAMESPACE,
        'get',
        'deployment/healthy',
        '-o',
        'jsonpath={.spec.replicas}',
      ]).trim(),
    ).toBe(String(starting));
    await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
    await expect(dialog).toBeHidden();
    expect(
      kubectl([
        '-n',
        NAMESPACE,
        'get',
        'deployment/healthy',
        '-o',
        'jsonpath={.spec.replicas}',
      ]).trim(),
    ).toBe(String(starting));
  } finally {
    kubectl(['-n', NAMESPACE, 'scale', 'deployment/healthy', `--replicas=${String(original)}`]);
    await close();
  }
});

test('the protected workload name permits scaling to zero and the apiserver records it', async ({
  browser,
}) => {
  const original = Number(
    kubectl([
      '-n',
      NAMESPACE,
      'get',
      'deployment/healthy',
      '-o',
      'jsonpath={.spec.replicas}',
    ]).trim(),
  );
  expect(Number.isInteger(original)).toBe(true);
  let starting = original;
  if (starting <= 0) {
    starting = 2;
  }
  kubectl(['-n', NAMESPACE, 'scale', 'deployment/healthy', `--replicas=${String(starting)}`]);
  const [page, close] = await openGuardedWorkload(browser);
  try {
    await page.getByRole('spinbutton', { name: 'replicas' }).fill('0');
    await page.getByRole('button', { name: 'Scale', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: 'Confirm on a protected cluster' });
    await dialog.getByRole('textbox', { name: 'Name', exact: true }).fill('healthy');
    await dialog.getByRole('button', { name: 'Confirm', exact: true }).click();
    await expect
      .poll(
        () =>
          kubectl([
            '-n',
            NAMESPACE,
            'get',
            'deployment/healthy',
            '-o',
            'jsonpath={.spec.replicas}',
          ]).trim(),
        { timeout: 60_000 },
      )
      .toBe('0');
  } finally {
    kubectl(['-n', NAMESPACE, 'scale', 'deployment/healthy', `--replicas=${String(original)}`]);
    await close();
  }
});

test('deleting on a protected cluster can be cancelled without touching the object', async ({
  browser,
}) => {
  const [page, close] = await openGuardedYaml(browser);
  try {
    await page.getByRole('button', { name: 'Delete', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: 'Confirm on a protected cluster' });
    await expect(dialog).toContainText(`Deleting ConfigMap ${GUARDED}.`);
    await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
    await expect(dialog).toBeHidden();
    expect(kubectlSoft(['-n', NAMESPACE, 'get', `configmap/${GUARDED}`])).toBe(0);
  } finally {
    if (kubectlSoft(['-n', NAMESPACE, 'get', `configmap/${GUARDED}`]) !== 0) {
      kubectl(['-n', NAMESPACE, 'create', 'configmap', GUARDED, '--from-literal=gate=after']);
    }
    await close();
  }
});

test('typing the protected object name permits its deletion', async ({ browser }) => {
  const [page, close] = await openGuardedYaml(browser);
  try {
    await page.getByRole('button', { name: 'Delete', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: 'Confirm on a protected cluster' });
    await dialog.getByRole('textbox', { name: 'Name', exact: true }).fill(GUARDED);
    await dialog.getByRole('button', { name: 'Confirm', exact: true }).click();
    await expect
      .poll(() => kubectlSoft(['-n', NAMESPACE, 'get', `configmap/${GUARDED}`]), {
        timeout: 60_000,
      })
      .not.toBe(0);
  } finally {
    kubectl(['-n', NAMESPACE, 'delete', 'configmap', GUARDED, '--ignore-not-found']);
    kubectl(['-n', NAMESPACE, 'create', 'configmap', GUARDED, '--from-literal=gate=before']);
    await close();
  }
});

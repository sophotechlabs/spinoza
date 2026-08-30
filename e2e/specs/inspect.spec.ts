import { expect, test } from '../harness/test';
import { openResource, selectRow } from '../harness/app';
import { kubectl } from '../harness/cluster';
import { NAMESPACE } from '../harness/paths';
import type { Page } from '@playwright/test';

test.describe.configure({ mode: 'serial' });

const EDITED = 'edited-by-e2e';

async function openYaml(page: Page, name: string): Promise<void> {
  await openResource(page, 'configmaps', 'ConfigMap');
  await selectRow(page, name);
  await page.getByRole('tab', { name: 'YAML', exact: true }).click();
  await expect(page.locator('.monaco-editor').first()).toBeVisible({ timeout: 60_000 });
}

async function editorText(page: Page): Promise<string> {
  const raw = await page.locator('.view-lines').first().innerText();
  return raw.replace(/\u00a0/g, ' ');
}

async function replaceEditor(page: Page, text: string): Promise<void> {
  await page.locator('.view-lines').first().click({ position: { x: 5, y: 5 } });
  await page.getByRole('textbox', { name: 'Editor content' }).focus();
  await page.keyboard.press('ControlOrMeta+a');
  await page.keyboard.press('Delete');
  await page.keyboard.insertText(text);
  await expect
    .poll(async () => (await editorText(page)).trim(), { timeout: 20_000 })
    .toBe(text.trim());
}

test.afterAll(() => {
  kubectl(['-n', NAMESPACE, 'delete', 'configmap', EDITED, '--ignore-not-found']);
});

test('the drawer offers every panel the object supports', async ({ page }) => {
  await openResource(page, 'pods', 'Pod');
  await selectRow(page, 'healthy-');
  const strip = page.getByRole('tablist', { name: 'right panels' });
  for (const tab of ['Overview', 'YAML', 'Events', 'Logs', 'Metrics']) {
    await expect(strip.getByRole('tab', { name: tab, exact: true })).toBeVisible();
  }
});

test('the editor shows the object the row selected, not a template', async ({ page }) => {
  await openYaml(page, 'config-sample');
  const text = await editorText(page);
  expect(text).toContain('kind: ConfigMap');
  expect(text).toContain('name: config-sample');
  expect(text).toContain(`namespace: ${NAMESPACE}`);
  expect(text).toContain('hello from e2e');
});

test('the editor carries what only the apiserver knows', async ({ page }) => {
  await openYaml(page, 'config-sample');
  const live = kubectl([
    '-n',
    NAMESPACE,
    'get',
    'configmap/config-sample',
    '-o',
    'jsonpath={.metadata.uid}',
  ]).trim();
  expect(await editorText(page)).toContain(live);
});

test('apply and revert stay disabled until the draft differs', async ({ page }) => {
  await openYaml(page, 'config-sample');
  await expect(page.getByRole('button', { name: 'Apply', exact: true })).toBeDisabled();
  await expect(page.getByRole('button', { name: 'Revert', exact: true })).toBeDisabled();
  await replaceEditor(page, 'apiVersion: v1');
  await expect(page.getByRole('button', { name: 'Apply', exact: true })).toBeEnabled();
  await expect(page.getByRole('button', { name: 'Revert', exact: true })).toBeEnabled();
});

test('reverting throws the draft away and puts the live object back', async ({ page }) => {
  await openYaml(page, 'config-sample');
  await replaceEditor(page, 'apiVersion: v1');
  await page.getByRole('button', { name: 'Revert', exact: true }).click();
  await expect
    .poll(async () => editorText(page), { timeout: 30_000 })
    .toContain('name: config-sample');
});

function liveVersion(name: string): string {
  return kubectl([
    '-n',
    NAMESPACE,
    'get',
    `configmap/${name}`,
    '-o',
    'jsonpath={.metadata.resourceVersion}',
  ]).trim();
}

function documentFor(
  name: string,
  resourceVersion: string | null,
  data: Record<string, string>,
): string {
  const metadata: Record<string, string> = { name, namespace: NAMESPACE };
  if (resourceVersion !== null) {
    metadata.resourceVersion = resourceVersion;
  }
  return JSON.stringify({ apiVersion: 'v1', kind: 'ConfigMap', metadata, data });
}

function alertOf(page: Page) {
  return page.getByRole('tabpanel', { name: 'YAML' }).getByRole('alert');
}

function dataOf(name: string, key: string): string {
  return kubectl([
    '-n',
    NAMESPACE,
    'get',
    `configmap/${name}`,
    '-o',
    `jsonpath={.data.${key}}`,
  ]).trim();
}

test('an apply that names no resourceVersion is refused, and the object is left alone', async ({
  page,
}) => {
  kubectl(['-n', NAMESPACE, 'create', 'configmap', EDITED, '--from-literal=before=yes']);
  await openYaml(page, EDITED);
  await replaceEditor(page, documentFor(EDITED, null, { before: 'yes', after: 'no-version' }));
  await page.getByRole('button', { name: 'Apply', exact: true }).click();

  await expect(alertOf(page)).toContainText('resourceVersion', { timeout: 30_000 });
  expect(dataOf(EDITED, 'after')).toBe('');
});

test('an edit that carries the resourceVersion reaches the apiserver', async ({ page }) => {
  await openYaml(page, EDITED);
  await replaceEditor(
    page,
    documentFor(EDITED, liveVersion(EDITED), { before: 'yes', after: 'also-yes' }),
  );
  await page.getByRole('button', { name: 'Apply', exact: true }).click();

  await expect(alertOf(page)).toBeEmpty({ timeout: 30_000 });
  await expect.poll(() => dataOf(EDITED, 'after'), { timeout: 60_000 }).toBe('also-yes');
});

test('an apply built on a read the cluster has moved past is refused', async ({ page }) => {
  const stale = liveVersion(EDITED);
  kubectl([
    '-n',
    NAMESPACE,
    'patch',
    `configmap/${EDITED}`,
    '--type=merge',
    '-p',
    '{"data":{"moved":"by-kubectl"}}',
  ]);
  expect(liveVersion(EDITED)).not.toBe(stale);

  await openYaml(page, EDITED);
  await replaceEditor(page, documentFor(EDITED, stale, { before: 'yes', after: 'stale-write' }));
  await page.getByRole('button', { name: 'Apply', exact: true }).click();

  await expect(alertOf(page)).not.toBeEmpty({ timeout: 30_000 });
  expect(dataOf(EDITED, 'after')).toBe('also-yes');
  expect(dataOf(EDITED, 'moved')).toBe('by-kubectl');
});

test('deleting from the drawer takes the object out of the table', async ({ page }) => {
  await openYaml(page, EDITED);
  await page.getByRole('button', { name: 'Delete', exact: true }).click();
  const confirm = page.getByRole('button', { name: 'Confirm', exact: true });
  await confirm.first().click({ timeout: 5_000 }).catch(() => undefined);
  await expect
    .poll(
      () =>
        kubectl([
          '-n',
          NAMESPACE,
          'get',
          'configmaps',
          '-o',
          'jsonpath={.items[*].metadata.name}',
        ]),
      { timeout: 60_000 },
    )
    .not.toContain(EDITED);
});

test('the events panel reports what the cluster recorded', async ({ page }) => {
  await openResource(page, 'pods', 'Pod');
  await selectRow(page, 'crashing-');
  await page.getByRole('tab', { name: 'Events', exact: true }).click();
  await expect(page.getByRole('tabpanel', { name: 'Events' })).toContainText(/BackOff|Pulled|Created/, {
    timeout: 60_000,
  });
});

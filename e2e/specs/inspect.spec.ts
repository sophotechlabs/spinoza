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
  const wanted = text.trim().split('\n')[0];
  await expect.poll(async () => (await editorText(page)).split('\n')[0], {
    timeout: 20_000,
  }).toBe(wanted);
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

test('an edit applied in the browser reaches the apiserver', async ({ page }) => {
  kubectl([
    '-n',
    NAMESPACE,
    'create',
    'configmap',
    EDITED,
    '--from-literal=before=yes',
  ]);
  await openYaml(page, EDITED);
  await expect.poll(async () => editorText(page), { timeout: 30_000 }).toContain(EDITED);
  const document = [
    'apiVersion: v1',
    'kind: ConfigMap',
    'metadata:',
    `  name: ${EDITED}`,
    `  namespace: ${NAMESPACE}`,
    'data:',
    '  before: yes',
    '  after: also-yes',
    '',
  ].join('\n');
  await replaceEditor(page, document);
  expect((await editorText(page)).trim()).toBe(document.trim());
  await page.getByRole('button', { name: 'Apply', exact: true }).click();
  const confirmApply = page.getByRole('button', { name: 'Confirm', exact: true });
  if ((await confirmApply.count()) > 0) {
    await confirmApply.first().click();
  }
  await expect(page.getByRole('tabpanel', { name: 'YAML' }).getByRole('alert')).toBeEmpty({
    timeout: 30_000,
  });
  await expect
    .poll(
      () =>
        kubectl([
          '-n',
          NAMESPACE,
          'get',
          `configmap/${EDITED}`,
          '-o',
          'jsonpath={.data.after}',
        ]).trim(),
      { timeout: 60_000 },
    )
    .toBe('also-yes');
});

test('deleting from the drawer takes the object out of the table', async ({ page }) => {
  await openYaml(page, EDITED);
  await page.getByRole('button', { name: 'Delete', exact: true }).click();
  const confirm = page.getByRole('button', { name: 'Confirm', exact: true });
  if ((await confirm.count()) > 0) {
    await confirm.first().click();
  }
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

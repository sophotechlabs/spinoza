import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '../harness/test';
import { openGrouped, openHome, openResource, openView, selectRow } from '../harness/app';
import type { Page } from '@playwright/test';

const TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'];

const VIEWS = [
  'issues',
  'topology',
  'helm',
  'checks',
  'history',
  'fleet',
  'rbac',
  'gitops',
  'flux-list',
  'flux-roles',
  'argo-apps',
  'argo-graph',
  'argo-list',
];

const SETTINGS_SECTIONS = ['Cluster', 'Columns', 'Logs', 'Terminal', 'Panels', 'Keyboard', 'About'];

async function settled(page: Page): Promise<void> {
  const main = page.locator('main');
  await expect(main).not.toBeEmpty({ timeout: 60_000 });
  await expect(main).not.toContainText('Loading', { timeout: 60_000 });
}

async function violations(page: Page): Promise<unknown[]> {
  const results = await new AxeBuilder({ page }).withTags(TAGS).analyze();
  return results.violations;
}

test('the cluster overview has no detectable accessibility violations', async ({ page }) => {
  await openHome(page);
  await settled(page);
  expect(await violations(page)).toEqual([]);
});

for (const view of VIEWS) {
  test(`the ${view} view has no detectable accessibility violations`, async ({ page }) => {
    await openView(page, view);
    await settled(page);
    expect(await violations(page)).toEqual([]);
  });
}

test('a resource table has no detectable accessibility violations', async ({ page }) => {
  await openResource(page, 'pods', 'Pod');
  await settled(page);
  expect(await violations(page)).toEqual([]);
});

test('a table discovery built for a custom type is no less accessible', async ({ page }) => {
  await openGrouped(page, 'spinoza.test', 'widgets', 'Widget');
  await settled(page);
  expect(await violations(page)).toEqual([]);
});

test('the inspect drawer has no detectable accessibility violations', async ({ page }) => {
  await openResource(page, 'pods', 'Pod');
  await selectRow(page, 'noshell');
  await page.getByRole('tab', { name: 'Overview', exact: true }).click();
  await expect(page.getByRole('tabpanel', { name: 'Overview' })).toBeVisible({ timeout: 60_000 });
  expect(await violations(page)).toEqual([]);
});

test('the yaml editor has no detectable accessibility violations', async ({ page }) => {
  await openResource(page, 'configmaps', 'ConfigMap');
  await selectRow(page, 'config-sample');
  await page.getByRole('tab', { name: 'YAML', exact: true }).click();
  await expect(page.locator('.monaco-editor').first()).toBeVisible({ timeout: 60_000 });
  expect(await violations(page)).toEqual([]);
});

test('the command palette has no detectable accessibility violations', async ({ page }) => {
  await openHome(page);
  await page.keyboard.press('ControlOrMeta+k');
  await expect(page.getByPlaceholder('Search')).toBeVisible();
  expect(await violations(page)).toEqual([]);
});

test('the settings dialog has no detectable accessibility violations', async ({ page }) => {
  await openHome(page);
  await page.getByRole('button', { name: 'Settings' }).click();
  await expect(page.getByRole('dialog', { name: 'Settings' })).toBeVisible();
  expect(await violations(page)).toEqual([]);
});

for (const section of SETTINGS_SECTIONS) {
  test(`the ${section} settings have no detectable accessibility violations`, async ({ page }) => {
    await openHome(page);
    await page.getByRole('button', { name: 'Settings' }).click();
    await page
      .getByRole('navigation', { name: 'Settings sections' })
      .getByRole('button', { name: section, exact: true })
      .click();
    await expect(page.getByRole('dialog', { name: 'Settings' })).toBeVisible();
    expect(await violations(page)).toEqual([]);
  });
}

test('the Helm install dialog has no detectable accessibility violations', async ({ page }) => {
  await openView(page, 'helm');
  await page.getByRole('button', { name: 'Install chart', exact: true }).click();
  await expect(page.getByRole('dialog', { name: 'Install a chart' })).toBeVisible({
    timeout: 60_000,
  });
  expect(await violations(page)).toEqual([]);
});

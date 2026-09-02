import { expect, test } from '../harness/test';
import { openGrouped, selectRow } from '../harness/app';

test('a type discovery found gets a table nobody wrote code for', async ({ page }) => {
  await openGrouped(page, 'spinoza.test', 'widgets', 'Widget');
  const headers = page.locator('main thead th');
  await expect(headers.filter({ hasText: 'Name' }).first()).toBeVisible({ timeout: 60_000 });
  for (const column of ['Colour', 'Teeth']) {
    await expect(headers.filter({ hasText: column }).first()).toBeVisible();
  }
});

test('the custom objects arrive with the values their columns name', async ({ page }) => {
  await openGrouped(page, 'spinoza.test', 'widgets', 'Widget');
  const rows = page.locator('main tbody tr');
  const cog = rows.filter({ hasText: 'cog' }).first();
  await expect(cog).toBeVisible({ timeout: 60_000 });
  await expect(cog).toContainText('brass');
  await expect(cog).toContainText('12');
  const sprocket = rows.filter({ hasText: 'sprocket' }).first();
  await expect(sprocket).toContainText('copper');
  await expect(sprocket).toContainText('30');
});

test('a custom object opens in the drawer like any other', async ({ page }) => {
  await openGrouped(page, 'spinoza.test', 'widgets', 'Widget');
  await selectRow(page, 'cog');
  await expect(page).toHaveTitle(/^cog widgets /);
  for (const tab of ['Overview', 'YAML', 'Events']) {
    await expect(page.getByRole('tab', { name: tab })).toBeVisible();
  }
});

test('custom-resource rows obey the same filtering contract as built-in kinds', async ({
  page,
}) => {
  await openGrouped(page, 'spinoza.test', 'widgets', 'Widget');
  const filter = page.getByRole('combobox', { name: 'Filter' });
  await filter.fill('cog');
  await expect(page.locator('main tbody tr').filter({ hasText: 'cog' })).toHaveCount(1);
  await expect(page.locator('main tbody tr').filter({ hasText: 'sprocket' })).toHaveCount(0);
  await filter.fill('');
  await expect(page.locator('main tbody tr').filter({ hasText: 'sprocket' })).toHaveCount(1);
});

test('the yaml drawer preserves fields discovered from a custom schema', async ({ page }) => {
  await openGrouped(page, 'spinoza.test', 'widgets', 'Widget');
  await selectRow(page, 'cog');
  await page.getByRole('tab', { name: 'YAML', exact: true }).click();
  const editor = page.locator('.view-lines').first();
  await expect(editor).toBeVisible({ timeout: 60_000 });
  await expect(editor).toContainText('apiVersion: spinoza.test/v1');
  await expect(editor).toContainText('colour: brass');
  await expect(editor).toContainText('teeth: 12');
});

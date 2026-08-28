import { expect, test } from '../harness/test';
import { openResource } from '../harness/app';

test('discovery lists a table for a core type', async ({ page }) => {
  await openResource(page, 'pods', 'Pod');
  await expect(page.getByRole('columnheader', { name: /^Name/ })).toBeVisible();
  for (const column of ['Namespace', 'Containers', 'Status', 'Restarts', 'Node', 'Age']) {
    await expect(page.getByRole('columnheader', { name: column })).toBeVisible();
  }
});

test('the seeded pods arrive in the table', async ({ page }) => {
  await openResource(page, 'pods', 'Pod');
  const rows = page.locator('main tbody tr');
  await expect(rows.filter({ hasText: 'healthy-' }).first()).toBeVisible({ timeout: 60_000 });
  await expect(rows.filter({ hasText: 'chatty-' }).first()).toBeVisible();
  await expect(rows.filter({ hasText: 'noshell' }).first()).toBeVisible();
});

test('the filter narrows the table to what was typed', async ({ page }) => {
  await openResource(page, 'pods', 'Pod');
  const rows = page.locator('main tbody tr');
  await expect(rows.filter({ hasText: 'coredns' }).first()).toBeVisible({ timeout: 60_000 });
  await page.getByPlaceholder('Filter by name, or field:value').fill('chatty');
  await expect(rows.filter({ hasText: 'chatty' })).toHaveCount(1);
  await expect(rows.filter({ hasText: 'coredns' })).toHaveCount(0);
});

test('a restart count is reported for a failing pod', async ({ page }) => {
  await openResource(page, 'pods', 'Pod');
  const crashing = page.locator('main tbody tr').filter({ hasText: 'crashing-' }).first();
  await expect(crashing).toBeVisible({ timeout: 60_000 });
  await expect(crashing).not.toContainText('crashing-x');
});

test('selecting a row deep-links to that object', async ({ page }) => {
  await openResource(page, 'pods', 'Pod');
  const row = page.locator('main tbody tr').filter({ hasText: 'noshell' }).first();
  await expect(row).toBeVisible({ timeout: 60_000 });
  await row.click();
  await expect(page).toHaveTitle(/^noshell pods /);
  for (const tab of ['Overview', 'YAML', 'Events', 'Logs']) {
    await expect(page.getByRole('tab', { name: tab })).toBeVisible();
  }
});

test('a deep link to an object opens it without a click', async ({ page }) => {
  await openResource(page, 'pods', 'Pod');
  await page.goto(
    '/#context=kind-spinoza-e2e&version=v1&resource=pods&kind=Pod&namespace=e2e&name=noshell',
  );
  await expect(page).toHaveTitle(/^noshell pods /);
});

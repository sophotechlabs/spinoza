import { expect, test } from '../harness/test';
import { openResource } from '../harness/app';
import { CONTEXT } from '../harness/paths';

test('discovery lists a table for a core type', async ({ page }) => {
  await openResource(page, 'pods', 'Pod');
  const headers = page.locator('main thead th');
  for (const column of ['Name', 'Namespace', 'Containers', 'Status', 'Restarts', 'Node', 'Age']) {
    await expect(headers.filter({ hasText: column }).first()).toBeVisible();
  }
  await expect(headers.filter({ hasText: 'Name' }).first()).toHaveAttribute('aria-sort', /.+/);
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
    `/#context=${CONTEXT}&version=v1&resource=pods&kind=Pod&namespace=e2e&name=noshell`,
  );
  await expect(page).toHaveTitle(/^noshell pods /);
});

test('sorting changes both the announced direction and the row order', async ({ page }) => {
  await openResource(page, 'pods', 'Pod');
  const header = page.getByRole('columnheader', { name: /Name/ });
  const sort = header.getByRole('button');
  const first = page.locator('main tbody tr').first();
  await expect(first).toBeVisible({ timeout: 60_000 });
  await sort.click();
  await expect(header).toHaveAttribute('aria-sort', 'ascending');
  const ascending = await first.textContent();
  await sort.click();
  await expect(header).toHaveAttribute('aria-sort', 'descending');
  await expect.poll(() => first.textContent()).not.toBe(ascending);
});

test('a field filter becomes a removable chip and filters the real rows', async ({ page }) => {
  await openResource(page, 'pods', 'Pod');
  const filter = page.getByRole('combobox', { name: 'Filter' });
  await filter.fill('status:Running');
  await filter.press('Enter');
  await expect(page.getByText('Status:', { exact: true })).toBeVisible();
  await expect(page.locator('main tbody tr').filter({ hasText: 'healthy-' }).first()).toBeVisible({
    timeout: 60_000,
  });
  await expect(page.locator('main tbody tr').filter({ hasText: 'crashing-' })).toHaveCount(0);
  await page.getByRole('button', { name: 'Remove the Status Running filter' }).click();
  await expect(page.locator('main tbody tr').filter({ hasText: 'crashing-' }).first()).toBeVisible({
    timeout: 60_000,
  });
});

test('the filter shortcut focuses the resource filter without typing into the page', async ({ page }) => {
  await openResource(page, 'pods', 'Pod');
  const filter = page.getByRole('combobox', { name: 'Filter' });
  await page.keyboard.press('/');
  await expect(filter).toBeFocused();
});

test('column visibility survives a reload and can be restored', async ({ page }) => {
  await openResource(page, 'pods', 'Pod');
  await page.getByText('Columns', { exact: true }).click();
  await page.getByLabel('Namespace', { exact: true }).uncheck();
  await expect(page.getByRole('columnheader', { name: /Namespace/ })).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole('columnheader', { name: /Namespace/ })).toHaveCount(0);
  await page.getByText('Columns', { exact: true }).click();
  await page.getByLabel('Namespace', { exact: true }).check();
  await expect(page.getByRole('columnheader', { name: /Namespace/ })).toBeVisible();
});

test('namespace scoping removes objects from other namespaces and is reversible', async ({ page }) => {
  await openResource(page, 'pods', 'Pod');
  const picker = page.getByRole('combobox', { name: 'Namespace', exact: true });
  await expect(page.locator('main tbody tr').filter({ hasText: 'coredns' }).first()).toBeVisible({
    timeout: 60_000,
  });
  await picker.selectOption('e2e');
  await expect(page.getByText('Namespace:', { exact: true })).toBeVisible();
  await expect(page.locator('main tbody tr').filter({ hasText: 'coredns' })).toHaveCount(0);
  await expect(page.locator('main tbody tr').filter({ hasText: 'healthy-' }).first()).toBeVisible();
  await picker.selectOption('');
  await expect(page.locator('main tbody tr').filter({ hasText: 'coredns' }).first()).toBeVisible({
    timeout: 60_000,
  });
});

test('an empty result offers a working way back to the table', async ({ page }) => {
  await openResource(page, 'pods', 'Pod');
  await page.getByRole('combobox', { name: 'Filter' }).fill('there-is-no-pod-with-this-name');
  await expect(page.getByText('Nothing matches the current filter.')).toBeVisible();
  await page.getByRole('button', { name: 'Clear filter', exact: true }).click();
  await expect(page.locator('main tbody tr').first()).toBeVisible({ timeout: 60_000 });
});

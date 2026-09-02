import { expect, test } from '../harness/test';
import { ensureDrawer, openResource } from '../harness/app';

const VALUE = 'e2e-password';

test('a secret value never reaches the page unasked', async ({ page }) => {
  await openResource(page, 'secrets', 'Secret');
  const row = page.locator('main tbody tr').filter({ hasText: 'secret-sample' }).first();
  await expect(row).toBeVisible({ timeout: 60_000 });
  await row.getByRole('button', { name: 'secret-sample', exact: true }).click();
  await ensureDrawer(page);
  await expect(page).toHaveTitle(/^secret-sample secrets /);
  await expect(page.locator('body')).not.toContainText(VALUE);
});

test('the key names are listed even while the values are held back', async ({ page }) => {
  await openResource(page, 'secrets', 'Secret');
  const row = page.locator('main tbody tr').filter({ hasText: 'secret-sample' }).first();
  await expect(row).toBeVisible({ timeout: 60_000 });
  await row.getByRole('button', { name: 'secret-sample', exact: true }).click();
  await ensureDrawer(page);
  await page.getByRole('tab', { name: 'Overview', exact: true }).click();
  await expect(page.getByRole('tabpanel', { name: 'Overview' })).toContainText('password', {
    timeout: 30_000,
  });
  await expect(page.locator('body')).not.toContainText(VALUE);
});

test('a configmap shows its data outright, because none of it is secret', async ({ page }) => {
  await openResource(page, 'configmaps', 'ConfigMap');
  const row = page.locator('main tbody tr').filter({ hasText: 'config-sample' }).first();
  await expect(row).toBeVisible({ timeout: 60_000 });
  await row.getByRole('button', { name: 'config-sample', exact: true }).click();
  await ensureDrawer(page);
  await page.getByRole('tab', { name: 'Overview', exact: true }).click();
  await expect(page.getByRole('tabpanel', { name: 'Overview' })).toContainText('greeting', {
    timeout: 30_000,
  });
});

test('one secret value can be revealed and hidden without exposing the others', async ({
  page,
}) => {
  await openResource(page, 'secrets', 'Secret');
  const row = page.locator('main tbody tr').filter({ hasText: 'secret-sample' }).first();
  await expect(row).toBeVisible({ timeout: 60_000 });
  await row.getByRole('button', { name: 'secret-sample', exact: true }).click();
  await ensureDrawer(page);
  await expect(page).toHaveTitle(/^secret-sample secrets /);
  await page.getByRole('tab', { name: 'Overview', exact: true }).click();
  const panel = page.getByRole('tabpanel', { name: 'Overview' });
  const password = panel.getByRole('textbox', { name: 'password' });
  const username = panel.getByRole('textbox', { name: 'username' });
  await expect(password).toHaveValue('••••••••••••');
  await expect(username).toHaveValue('••••••••••••');
  await panel.getByRole('button', { name: 'Show password' }).click();
  await expect(password).toHaveValue(VALUE);
  await expect(username).toHaveValue('••••••••••••');
  await panel.getByRole('button', { name: 'Hide password' }).click();
  await expect(password).toHaveValue('••••••••••••');
});

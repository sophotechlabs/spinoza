import { expect, test } from '../harness/test';
import { ensureDrawer, openResource } from '../harness/app';

const VALUE = 'e2e-password';

test('a secret value never reaches the page unasked', async ({ page }) => {
  await openResource(page, 'secrets', 'Secret');
  const row = page.locator('main tbody tr').filter({ hasText: 'secret-sample' }).first();
  await expect(row).toBeVisible({ timeout: 60_000 });
  await row.locator('button').first().click();
  await ensureDrawer(page);
  await expect(page).toHaveTitle(/^secret-sample secrets /);
  await expect(page.locator('body')).not.toContainText(VALUE);
});

test('the key names are listed even while the values are held back', async ({ page }) => {
  await openResource(page, 'secrets', 'Secret');
  const row = page.locator('main tbody tr').filter({ hasText: 'secret-sample' }).first();
  await expect(row).toBeVisible({ timeout: 60_000 });
  await row.locator('button').first().click();
  await ensureDrawer(page);
  await expect(page.locator('main')).toContainText('password', { timeout: 30_000 });
  await expect(page.locator('body')).not.toContainText(VALUE);
});

test('a configmap shows its data outright, because none of it is secret', async ({ page }) => {
  await openResource(page, 'configmaps', 'ConfigMap');
  const row = page.locator('main tbody tr').filter({ hasText: 'config-sample' }).first();
  await expect(row).toBeVisible({ timeout: 60_000 });
  await row.locator('button').first().click();
  await ensureDrawer(page);
  await expect(page.locator('main')).toContainText('greeting', { timeout: 30_000 });
});

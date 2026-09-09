import { expect, test } from '../harness/test';
import { openGrouped, openPalette } from '../harness/app';

const NAME = 'e2e saved view';

async function forgetAll(page: import('@playwright/test').Page): Promise<void> {
  await page.evaluate(async () => {
    const response = await fetch('/api/views');
    const page_ = (await response.json()) as { views: { id: string; shared?: boolean }[] };
    for (const one of page_.views) {
      const params = new URLSearchParams({ id: one.id });
      if (one.shared === true) {
        params.set('shared', 'true');
      }
      await fetch(`/api/views?${params.toString()}`, { method: 'DELETE' });
    }
  });
}

test.describe('saved views', () => {
  test.afterEach(async ({ page }) => {
    await forgetAll(page);
  });

  test('keeps a filtered table under a name and offers it in the palette', async ({ page }) => {
    await openGrouped(page, '', 'configmaps', 'ConfigMap');
    await expect(page.locator('main tbody tr').first()).toBeVisible({ timeout: 120_000 });

    await page.getByRole('button', { name: 'Save this view' }).click();
    await page.getByLabel('Name for this view').fill(NAME);
    await page.getByRole('button', { name: 'Save', exact: true }).click();

    await expect(page.getByText(new RegExp(`saved "${NAME}"`))).toBeVisible({ timeout: 30_000 });

    await openPalette(page);
    await expect(page.getByRole('button', { name: new RegExp(NAME) })).toBeVisible({
      timeout: 30_000,
    });
  });

  test('refuses a view with no name', async ({ page }) => {
    await openGrouped(page, '', 'configmaps', 'ConfigMap');
    await expect(page.locator('main tbody tr').first()).toBeVisible({ timeout: 120_000 });

    await page.getByRole('button', { name: 'Save this view' }).click();

    await expect(page.getByRole('button', { name: 'Save', exact: true })).toBeDisabled();
  });

  test('opening a saved view puts the kind back', async ({ page }) => {
    await openGrouped(page, '', 'configmaps', 'ConfigMap');
    await expect(page.locator('main tbody tr').first()).toBeVisible({ timeout: 120_000 });
    await page.getByRole('button', { name: 'Save this view' }).click();
    await page.getByLabel('Name for this view').fill(NAME);
    await page.getByRole('button', { name: 'Save', exact: true }).click();
    await expect(page.getByText(new RegExp(`saved "${NAME}"`))).toBeVisible({ timeout: 30_000 });

    await openGrouped(page, '', 'secrets', 'Secret');
    await openPalette(page);
    await page.getByRole('button', { name: new RegExp(NAME) }).click();

    await expect(page.getByRole('heading', { name: /ConfigMap resources/ })).toBeVisible({
      timeout: 60_000,
    });
  });
});

import { expect, test } from '../harness/test';
import { openView } from '../harness/app';

test.describe('reserved against used', () => {
  test('ranks namespaces by what they hold and never use', async ({ page }) => {
    await openView(page, 'waste');

    await expect(page.getByRole('heading', { name: 'Reserved against used' })).toBeVisible({
      timeout: 120_000,
    });
    await expect(page.getByRole('columnheader', { name: 'Namespace' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'CPU asked' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'CPU spare' })).toBeVisible();
    await expect(page.locator('main tbody tr').first()).toBeVisible();
  });

  test('groups by workload as well as by namespace', async ({ page }) => {
    await openView(page, 'waste');
    await page.getByRole('button', { name: 'By workload' }).click();

    await expect(page.getByRole('columnheader', { name: 'Workload' })).toBeVisible({
      timeout: 120_000,
    });
  });

  test('says what it could not measure rather than drawing a zero', async ({ page }) => {
    await openView(page, 'waste');
    await expect(page.locator('main tbody tr').first()).toBeVisible({ timeout: 120_000 });

    const measured = await page.evaluate(async () => {
      const response = await fetch('/api/waste');
      return (await response.json()) as {
        measured?: boolean;
        reason?: string;
        source?: string;
        window?: string;
      };
    });

    if (measured.measured === true) {
      expect(measured.source ?? '').not.toBe('');
      expect(measured.window ?? '').not.toBe('');
    } else {
      expect(measured.reason ?? '').not.toBe('');
      await expect(page.getByText('not measured').first()).toBeVisible();
    }
  });
});

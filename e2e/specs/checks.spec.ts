import { expect, test } from '../harness/test';
import { openView } from '../harness/app';

test('the scan reports findings across every family', async ({ page }) => {
  await openView(page, 'checks');
  const main = page.locator('main');
  await expect(main).toContainText(/\d+ findings across \d+ workloads/, { timeout: 60_000 });
  await expect(page.getByRole('heading', { name: 'Security' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Reliability' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Efficiency' })).toBeVisible();
});

test('the seeded workload trips the checks it was built to trip', async ({ page }) => {
  await openView(page, 'checks');
  const main = page.locator('main');
  await expect(main).toContainText('Privileged containers', { timeout: 60_000 });
  await expect(main).toContainText('Host namespaces shared');
  await expect(main).toContainText('Dangerous capabilities added');
});

test('a check nothing trips is reported clean rather than hidden', async ({ page }) => {
  await openView(page, 'checks');
  await expect(page.locator('main')).toContainText('clean', { timeout: 60_000 });
});

test('findings carry the framework that asks for them', async ({ page }) => {
  await openView(page, 'checks');
  const main = page.locator('main');
  await expect(main).toContainText('NSA/CISA', { timeout: 60_000 });
  await expect(main).toContainText('PSS');
});

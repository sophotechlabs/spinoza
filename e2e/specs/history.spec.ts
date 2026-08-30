import { expect, test } from '../harness/test';
import { openGrouped, openView, selectRow } from '../harness/app';
import { kubectl } from '../harness/cluster';
import { NAMESPACE } from '../harness/paths';

test.describe.configure({ mode: 'serial' });

test('the view says what it is for', async ({ page }) => {
  await openView(page, 'history');
  const showing = page.getByRole('combobox', { name: 'What to show' });
  await expect(showing).toBeVisible({ timeout: 60_000 });
  await expect(showing).toContainText('What I did');
  await expect(showing).toContainText('What changed');
  await expect(page.getByRole('combobox', { name: 'What to record' })).toBeVisible();
});

test('a write made in the browser turns up in the history', async ({ page }) => {
  await openGrouped(page, 'apps', 'deployments', 'Deployment');
  await selectRow(page, 'chatty');
  await page.getByRole('tab', { name: 'Overview', exact: true }).click();
  const replicas = page.getByRole('spinbutton', { name: 'replicas' });
  await expect(replicas).toBeVisible({ timeout: 30_000 });
  await replicas.fill('3');
  await page.getByRole('button', { name: 'Scale', exact: true }).click();
  await expect
    .poll(
      () =>
        kubectl([
          '-n',
          NAMESPACE,
          'get',
          'deployment/chatty',
          '-o',
          'jsonpath={.spec.replicas}',
        ]).trim(),
      { timeout: 60_000 },
    )
    .toBe('3');
  await openView(page, 'history');
  await expect(page.locator('main')).toContainText('chatty', { timeout: 60_000 });
});

test('the history names the object and what was done to it', async ({ page }) => {
  await openView(page, 'history');
  const main = page.locator('main');
  await expect(main).toContainText('chatty', { timeout: 60_000 });
  await expect(main).toContainText('scale');
  await expect(main).toContainText('deployments');
  for (const column of ['When', 'Did', 'To', 'Namespace', 'Outcome']) {
    await expect(main).toContainText(column);
  }
});

test('clearing the history empties it and says so', async ({ page }) => {
  await openView(page, 'history');
  await expect(page.locator('main')).toContainText('chatty', { timeout: 60_000 });
  const clear = page.getByRole('button', { name: 'Clear', exact: true });
  await expect(clear).toBeEnabled();
  await clear.click();
  await expect(page.locator('main')).toContainText('There is nothing here yet.', {
    timeout: 30_000,
  });
  await expect(clear).toBeDisabled();
});

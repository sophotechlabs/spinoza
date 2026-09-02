import { expect, test } from '../harness/test';
import { openHome, openView } from '../harness/app';
import { openSecondCluster } from '../harness/multicluster';
import { CONTEXT, SECOND_CONTEXT } from '../harness/paths';

async function openFleet(page: import('@playwright/test').Page): Promise<void> {
  await openHome(page);
  await openSecondCluster(page);
  await openView(page, 'fleet');
}

test('fleet exposes every promised cross-cluster inventory', async ({ page }) => {
  await openFleet(page);
  for (const tab of ['Clusters', 'What is on them', 'Releases', 'Delivery', 'Images']) {
    await expect(page.getByRole('button', { name: tab, exact: true })).toBeVisible();
  }
});

test('fleet overview reports both open clusters and its total row', async ({ page }) => {
  await openFleet(page);
  const table = page.locator('main table');
  await expect(table).toContainText(CONTEXT, { timeout: 90_000 });
  await expect(table).toContainText(SECOND_CONTEXT);
  await expect(table).toContainText('Everything open');
});

test('fleet inventory counts resource kinds per cluster', async ({ page }) => {
  await openFleet(page);
  await page.getByRole('button', { name: 'What is on them', exact: true }).click();
  await expect(page.locator('main').getByText('deployments', { exact: true })).toBeVisible({
    timeout: 90_000,
  });
  await expect(page.getByText(new RegExp(CONTEXT)).first()).toBeVisible();
});

test('fleet releases preserve the owning cluster and Helm release', async ({ page }) => {
  await openFleet(page);
  await page.getByRole('button', { name: 'Releases', exact: true }).click();
  await expect(page.getByText('e2e-release', { exact: true })).toBeVisible({ timeout: 90_000 });
  await expect(page.getByText(new RegExp(CONTEXT)).first()).toBeVisible();
});

test('fleet delivery combines Flux and Argo without losing cluster provenance', async ({ page }) => {
  await openFleet(page);
  await page.getByRole('button', { name: 'Delivery', exact: true }).click();
  await expect(page.getByText(/Flux|Argo/).first()).toBeVisible({ timeout: 90_000 });
  await expect(page.getByText(new RegExp(CONTEXT)).first()).toBeVisible();
});

test('fleet images report image use and version skew across clusters', async ({ page }) => {
  await openFleet(page);
  await page.getByRole('button', { name: 'Images', exact: true }).click();
  await expect(page.getByText(/busybox/).first()).toBeVisible({ timeout: 90_000 });
  await expect(page.getByText(/pods/).first()).toBeVisible();
});

test('choosing a fleet cluster activates that cluster', async ({ page }) => {
  await openFleet(page);
  await page.getByRole('button', { name: new RegExp(SECOND_CONTEXT) }).first().click();
  await expect(page.getByRole('banner')).toContainText(SECOND_CONTEXT, { timeout: 90_000 });
});

import { expect, test } from '../harness/test';
import { openView } from '../harness/app';
import { helm } from '../harness/cluster';
import { NAMESPACE } from '../harness/paths';
import { RELEASE } from '../harness/fixtures';
import type { Page } from '@playwright/test';

test.describe.configure({ mode: 'serial' });

async function openRelease(page: Page): Promise<void> {
  await openView(page, 'helm');
  const row = page.locator('main tbody tr').filter({ hasText: RELEASE }).first();
  await expect(row).toBeVisible({ timeout: 60_000 });
  await row.getByRole('button', { name: RELEASE, exact: true }).click();
  const tab = page.getByRole('tab', { name: 'Release', exact: true });
  await expect(tab).toBeEnabled({ timeout: 60_000 });
  await tab.click();
  await expect(page.getByRole('tabpanel', { name: 'Release' })).toBeVisible({ timeout: 60_000 });
}

function panel(page: Page) {
  return page.getByRole('tabpanel', { name: 'Release' });
}

async function openTab(page: Page, name: string): Promise<void> {
  const chip = panel(page).getByRole('button', { name, exact: true });
  await chip.click();
  await expect(chip).toHaveAttribute('aria-pressed', 'true');
}

test('a release helm installed is read out of helm own storage', async ({ page }) => {
  await openView(page, 'helm');
  const row = page.locator('main tbody tr').filter({ hasText: RELEASE }).first();
  await expect(row).toBeVisible({ timeout: 60_000 });
  await expect(row).toContainText('spinoza-e2e');
  await expect(row).toContainText(NAMESPACE);
});

test('the count is reported honestly', async ({ page }) => {
  await openView(page, 'helm');
  await expect(page.locator('main')).toContainText(/1 of 1/, { timeout: 60_000 });
});

test('a chart no repository carries is said to be unknown, not guessed at', async ({ page }) => {
  await openView(page, 'helm');
  const row = page.locator('main tbody tr').filter({ hasText: RELEASE }).first();
  await expect(row).toContainText('no chart repository knows this chart', { timeout: 60_000 });
});

test('the table names what helm itself records about a release', async ({ page }) => {
  await openView(page, 'helm');
  const headers = page.locator('main thead th');
  for (const column of ['Name', 'Namespace', 'Chart', 'Chart version', 'App version', 'Rev', 'Status']) {
    await expect(headers.filter({ hasText: column }).first()).toBeVisible({ timeout: 60_000 });
  }
  const row = page.locator('main tbody tr').filter({ hasText: RELEASE }).first();
  await expect(row).toContainText('deployed');
  await expect(row).toContainText('1.0.0');
});

test('installing a chart is offered when helm and the cluster both allow it', async ({ page }) => {
  await openView(page, 'helm');
  await expect(page.getByRole('button', { name: 'Install chart' })).toBeEnabled({
    timeout: 60_000,
  });
});

test('the release detail offers every tab it promises', async ({ page }) => {
  await openRelease(page);
  for (const tab of ['Overview', 'Values', 'Notes', 'Manifest', 'Resources', 'History']) {
    await expect(panel(page).getByRole('button', { name: tab, exact: true })).toBeVisible();
  }
});

test('the values are the ones the release was installed with', async ({ page }) => {
  await openRelease(page);
  await openTab(page, 'Values');
  await expect(panel(page)).toContainText('hello from revision two', { timeout: 30_000 });
});

test('the notes are the ones the chart rendered', async ({ page }) => {
  await openRelease(page);
  await openTab(page, 'Notes');
  await expect(panel(page)).toContainText(RELEASE, { timeout: 30_000 });
  await expect(panel(page)).toContainText('spinoza-e2e chart is installed');
});

test('the manifest is what helm actually put in the cluster', async ({ page }) => {
  await openRelease(page);
  await openTab(page, 'Manifest');
  await expect(panel(page)).toContainText('kind: Deployment', { timeout: 30_000 });
  await expect(panel(page)).toContainText(`${RELEASE}-greeting`);
});

test('the resources are the objects the release rendered', async ({ page }) => {
  await openRelease(page);
  await openTab(page, 'Resources');
  await expect(panel(page)).toContainText('Deployment', { timeout: 30_000 });
  await expect(panel(page)).toContainText('ConfigMap');
});

test('the history carries every revision helm recorded', async ({ page }) => {
  const recorded = JSON.parse(
    helm(['history', RELEASE, '--namespace', NAMESPACE, '-o', 'json']),
  ) as { revision: number }[];
  expect(recorded.length).toBeGreaterThan(1);
  const latest = recorded[recorded.length - 1].revision;
  await openRelease(page);
  await openTab(page, 'History');
  await expect(panel(page)).toContainText(String(latest), { timeout: 30_000 });
  await expect(panel(page)).toContainText(String(latest - 1));
});

test('the selected release travels in the url through a reload', async ({ page }) => {
  await openRelease(page);
  expect(page.url()).toContain(`release=${RELEASE}`);
  expect(page.url()).toContain(`releaseNs=${NAMESPACE}`);
  await page.reload();
  await page.waitForLoadState('domcontentloaded');
  await expect(panel(page)).toBeVisible({ timeout: 60_000 });
  await expect(panel(page)).toContainText(RELEASE);
});

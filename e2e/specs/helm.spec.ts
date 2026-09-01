import { expect, test } from '../harness/test';
import { openView } from '../harness/app';
import { helm } from '../harness/cluster';
import { NAMESPACE } from '../harness/paths';
import { DOOMED, RELEASE } from '../harness/fixtures';
import { NEXT_VERSION, REPO_NAME } from '../harness/charts';
import type { Locator, Page } from '@playwright/test';

async function openNamed(page: Page, release: string): Promise<void> {
  await openView(page, 'helm');
  const row = page.locator('main tbody tr').filter({ hasText: release }).first();
  await expect(row).toBeVisible({ timeout: 60_000 });
  await row.getByRole('button', { name: release, exact: true }).click();
  const tab = page.getByRole('tab', { name: 'Release', exact: true });
  await expect(tab).toBeEnabled({ timeout: 60_000 });
  await tab.click();
  await expect(page.getByRole('tabpanel', { name: 'Release' })).toBeVisible({ timeout: 60_000 });
}

async function openRelease(page: Page): Promise<void> {
  await openNamed(page, RELEASE);
}

function panel(page: Page) {
  return page.getByRole('tabpanel', { name: 'Release' });
}

async function previewed(dialog: Locator): Promise<void> {
  await expect
    .poll(
      async () => {
        const alert = (await dialog.getByRole('alert').innerText()).trim();
        if (alert !== '') {
          return alert;
        }
        const ready = await dialog
          .getByRole('button', { name: `Upgrade to ${NEXT_VERSION}` })
          .count();
        if (ready > 0) {
          return 'ready';
        }
        return '';
      },
      { timeout: 120_000 },
    )
    .toBe('ready');
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

test('the count is the number helm itself would report', async ({ page }) => {
  const installed = (
    JSON.parse(helm(['list', '--namespace', NAMESPACE, '-o', 'json'])) as unknown[]
  ).length;
  await openView(page, 'helm');
  await expect(page.locator('main')).toContainText(
    new RegExp(`${String(installed)} of ${String(installed)}`),
    { timeout: 60_000 },
  );
});

test('the newest version the repo offers is reported beside the installed one', async ({
  page,
}) => {
  await openView(page, 'helm');
  const row = page.locator('main tbody tr').filter({ hasText: RELEASE }).first();
  await expect(row).toContainText(NEXT_VERSION, { timeout: 60_000 });
  await expect(row).not.toContainText('no chart repository knows this chart');
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

test('rolling back to the revision before puts its values back', async ({ page }) => {
  const before = helm(['get', 'values', RELEASE, '--namespace', NAMESPACE]);
  expect(before).toContain('hello from revision two');
  const recorded = JSON.parse(
    helm(['history', RELEASE, '--namespace', NAMESPACE, '-o', 'json']),
  ) as { revision: number }[];
  const target = recorded[recorded.length - 2].revision;

  await openRelease(page);
  await openTab(page, 'History');
  const rollback = panel(page).getByTitle(`Roll back to revision ${String(target)}`, {
    exact: true,
  });
  await expect(rollback).toBeEnabled({ timeout: 30_000 });
  await rollback.click();
  const confirm = page.getByRole('button', { name: 'Confirm', exact: true });
  await confirm.first().click({ timeout: 5_000 }).catch(() => undefined);

  await expect
    .poll(() => helm(['get', 'values', RELEASE, '--namespace', NAMESPACE]), { timeout: 90_000 })
    .not.toContain('hello from revision two');
});

test('the rollback is recorded as another revision, not a rewrite of history', async ({ page }) => {
  const recorded = JSON.parse(
    helm(['history', RELEASE, '--namespace', NAMESPACE, '-o', 'json']),
  ) as { revision: number; description: string }[];
  const latest = recorded[recorded.length - 1];
  expect(latest.description.toLowerCase()).toContain('rollback');

  await openRelease(page);
  await openTab(page, 'History');
  await expect(panel(page)).toContainText(String(latest.revision), { timeout: 30_000 });
});

test('uninstalling asks first, and then the release is gone', async ({ page }) => {
  await openNamed(page, DOOMED);
  await page.getByRole('button', { name: 'Uninstall', exact: true }).click();
  await expect(panel(page)).toContainText(`Uninstall ${DOOMED}? This cannot be undone.`, {
    timeout: 30_000,
  });
  await panel(page).getByRole('button', { name: 'Confirm', exact: true }).click();

  await expect
    .poll(() => helm(['list', '--namespace', NAMESPACE, '-o', 'json']), { timeout: 90_000 })
    .not.toContain(DOOMED);
  await openView(page, 'helm');
  await expect(page.locator('main tbody')).not.toContainText(DOOMED, { timeout: 60_000 });
});

test('a chart the repo offers is searchable and installable from the dialog', async ({ page }) => {
  await openView(page, 'helm');
  await page.getByRole('button', { name: 'Install chart' }).click();
  const dialog = page.getByRole('dialog', { name: 'Install a chart' });
  await dialog.getByRole('searchbox', { name: 'Search charts' }).fill('spinoza');
  await expect(dialog).toContainText('1 charts', { timeout: 60_000 });
  await expect(
    dialog.getByRole('button', { name: `spinoza-e2e ${NEXT_VERSION} from ${REPO_NAME}` }),
  ).toBeVisible();
});

test('the upgrade dialog offers the version the repo carries, not the one installed', async ({
  page,
}) => {
  await openRelease(page);
  await panel(page).getByRole('button', { name: 'Upgrade', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: `Upgrade ${RELEASE}` });
  const versions = dialog.getByRole('combobox', { name: 'Chart version' });
  await expect(versions.locator(`option[value$=":${NEXT_VERSION}"]`)).toHaveCount(1, {
    timeout: 60_000,
  });
  await expect(dialog).toContainText('from 0.1.0');
});

test('an upgrade renders the manifest it would apply before applying it', async ({ page }) => {
  await openRelease(page);
  await panel(page).getByRole('button', { name: 'Upgrade', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: `Upgrade ${RELEASE}` });
  const versions = dialog.getByRole('combobox', { name: 'Chart version' });
  await expect(versions.locator(`option[value$=":${NEXT_VERSION}"]`)).toHaveCount(1, {
    timeout: 60_000,
  });
  await versions.selectOption({ label: NEXT_VERSION });
  const preview = dialog.getByRole('button', { name: 'Preview', exact: true });
  await expect(preview).toBeEnabled({ timeout: 30_000 });
  await preview.click();

  await previewed(dialog);
  await expect(dialog.getByRole('button', { name: 'Back', exact: true })).toBeVisible();
  await expect(dialog.locator('.monaco-diff-editor').first()).toBeVisible({ timeout: 60_000 });
});

test('going through with the upgrade moves the release onto the new chart', async ({ page }) => {
  const before = JSON.parse(
    helm(['list', '--namespace', NAMESPACE, '--filter', RELEASE, '-o', 'json']),
  ) as { chart: string }[];
  expect(before[0].chart).toContain('0.1.0');

  await openRelease(page);
  await panel(page).getByRole('button', { name: 'Upgrade', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: `Upgrade ${RELEASE}` });
  const versions = dialog.getByRole('combobox', { name: 'Chart version' });
  await expect(versions.locator(`option[value$=":${NEXT_VERSION}"]`)).toHaveCount(1, {
    timeout: 60_000,
  });
  await versions.selectOption({ label: NEXT_VERSION });
  const preview = dialog.getByRole('button', { name: 'Preview', exact: true });
  await expect(preview).toBeEnabled({ timeout: 30_000 });
  await preview.click();
  await previewed(dialog);
  await dialog.getByRole('button', { name: `Upgrade to ${NEXT_VERSION}` }).click();
  const confirm = dialog.getByRole('button', { name: 'Confirm', exact: true });
  await confirm.first().click({ timeout: 5_000 }).catch(() => undefined);

  await expect
    .poll(
      () =>
        (
          JSON.parse(
            helm(['list', '--namespace', NAMESPACE, '--filter', RELEASE, '-o', 'json']),
          ) as { chart: string }[]
        )[0].chart,
      { timeout: 120_000 },
    )
    .toContain(NEXT_VERSION);
});

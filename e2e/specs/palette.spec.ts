import { expect, test } from '../harness/test';
import { openHome, openPalette } from '../harness/app';
import { primaryShortcut } from '../harness/keyboard';

const SEARCH = 'Search resources, views and recent objects';

test('the palette opens on its shortcut and closes on escape', async ({ page }) => {
  await openHome(page);
  await primaryShortcut(page, 'k');
  const box = page.getByRole('textbox', { name: SEARCH });
  await expect(box).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(box).toBeHidden();
});

test('the palette offers views and types before anything is typed', async ({ page }) => {
  await openHome(page);
  await openPalette(page);
  const palette = page.getByRole('dialog', { name: 'Command palette' });
  await expect(palette.getByRole('button', { name: 'Issues view' })).toBeVisible();
  await expect(palette.getByRole('button', { name: /^Pod Workloads/ })).toBeVisible();
});

test('typing narrows the palette to objects the cluster actually holds', async ({ page }) => {
  await openHome(page);
  await openPalette(page);
  await page.getByRole('textbox', { name: SEARCH }).fill('healthy');
  const palette = page.getByRole('dialog', { name: 'Command palette' });
  await expect(palette.getByRole('button', { name: 'e2e/healthy deployment' })).toBeVisible({
    timeout: 30_000,
  });
  await expect(
    palette.getByRole('button', { name: /^e2e\/healthy-.* pod$/ }).first(),
  ).toBeVisible();
  await expect(palette.getByRole('button', { name: 'e2e/healthy service' })).toBeVisible();
});

test('a search that matches nothing offers nothing rather than everything', async ({ page }) => {
  await openHome(page);
  await openPalette(page);
  await page.getByRole('textbox', { name: SEARCH }).fill('nothing-matches-this-at-all');
  const palette = page.getByRole('dialog', { name: 'Command palette' });
  await expect(palette.getByRole('button')).toHaveCount(0, { timeout: 30_000 });
});

test('choosing a view from the palette goes there', async ({ page }) => {
  await openHome(page);
  await openPalette(page);
  await page
    .getByRole('dialog', { name: 'Command palette' })
    .getByRole('button', { name: 'Cluster checks view' })
    .click();
  await expect(page).toHaveTitle(/^checks /, { timeout: 60_000 });
});

test('choosing an object from the palette opens it', async ({ page }) => {
  await openHome(page);
  await openPalette(page);
  await page.getByRole('textbox', { name: SEARCH }).fill('healthy');
  const palette = page.getByRole('dialog', { name: 'Command palette' });
  const hit = palette.getByRole('button', { name: 'e2e/healthy deployment' });
  await expect(hit).toBeVisible({ timeout: 30_000 });
  await hit.click();
  await expect(page).toHaveTitle(/^healthy deployments /, { timeout: 60_000 });
});

test('an empty palette result explains itself', async ({ page }) => {
  await openHome(page);
  await openPalette(page);
  await page.getByRole('textbox', { name: SEARCH }).fill('nothing-matches-this-at-all');
  await expect(page.getByText('Nothing matches that.', { exact: true })).toBeVisible({
    timeout: 30_000,
  });
});

test('choosing a discovered kind opens its live resource table', async ({ page }) => {
  await openHome(page);
  await openPalette(page);
  const box = page.getByRole('textbox', { name: SEARCH });
  await box.fill('Deployment');
  const kind = page
    .getByRole('dialog', { name: 'Command palette' })
    .getByRole('button', { name: /^Deployment Workloads apps\/v1$/ });
  await expect(kind).toBeVisible({ timeout: 30_000 });
  await kind.click();
  await expect(page).toHaveTitle(/^deployments /, { timeout: 60_000 });
  await expect(page.locator('main tbody tr').first()).toBeVisible({ timeout: 60_000 });
});

test('enter activates the filtered palette result without a mouse', async ({ page }) => {
  await openHome(page);
  await openPalette(page);
  const box = page.getByRole('textbox', { name: SEARCH });
  await box.fill('Cluster checks');
  await box.press('Enter');
  await expect(page).toHaveTitle(/^checks /, { timeout: 60_000 });
});

test('the palette remembers a dismissed query and selects it on reopening', async ({ page }) => {
  await openHome(page);
  await openPalette(page);
  const box = page.getByRole('textbox', { name: SEARCH });
  await box.fill('deploy');
  await box.press('Escape');
  await openPalette(page);
  await expect(box).toHaveValue('deploy');
  const selection = await box.evaluate((element) => {
    const input = element as HTMLInputElement;
    return [input.selectionStart, input.selectionEnd];
  });
  expect(selection).toEqual([0, 6]);
});

test('an opened object becomes a recent result on the next opening', async ({ page }) => {
  await openHome(page);
  await openPalette(page);
  const box = page.getByRole('textbox', { name: SEARCH });
  await box.fill('healthy');
  const palette = page.getByRole('dialog', { name: 'Command palette' });
  await palette.getByRole('button', { name: 'e2e/healthy deployment' }).click();
  await expect(page).toHaveTitle(/^healthy deployments /, { timeout: 60_000 });
  await openPalette(page);
  const recents = palette.getByRole('region', { name: 'Recent objects' });
  await expect(recents.getByRole('button', { name: 'e2e/healthy deployments' })).toBeVisible();
});

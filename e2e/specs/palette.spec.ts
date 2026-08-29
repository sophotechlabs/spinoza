import { expect, test } from '../harness/test';
import { openHome } from '../harness/app';

const SEARCH = 'Search resources, views and recent objects';

test('the palette opens on its shortcut and closes on escape', async ({ page }) => {
  await openHome(page);
  await page.keyboard.press('ControlOrMeta+k');
  const box = page.getByRole('textbox', { name: SEARCH });
  await expect(box).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(box).toBeHidden();
});

test('the palette offers views and types before anything is typed', async ({ page }) => {
  await openHome(page);
  await page.keyboard.press('ControlOrMeta+k');
  const palette = page.getByRole('dialog', { name: 'Command palette' });
  await expect(palette.getByRole('button', { name: 'Issues view' })).toBeVisible();
  await expect(palette.getByRole('button', { name: /^Pod Workloads/ })).toBeVisible();
});

test('typing narrows the palette to objects the cluster actually holds', async ({ page }) => {
  await openHome(page);
  await page.keyboard.press('ControlOrMeta+k');
  await page.getByRole('textbox', { name: SEARCH }).fill('healthy');
  const palette = page.getByRole('dialog', { name: 'Command palette' });
  await expect(palette.getByRole('button', { name: 'e2e/healthy deployment' })).toBeVisible({
    timeout: 30_000,
  });
  await expect(palette.getByRole('button', { name: /^e2e\/healthy-.* pod$/ }).first()).toBeVisible();
  await expect(palette.getByRole('button', { name: 'e2e/healthy service' })).toBeVisible();
});

test('a search that matches nothing offers nothing rather than everything', async ({ page }) => {
  await openHome(page);
  await page.keyboard.press('ControlOrMeta+k');
  await page.getByRole('textbox', { name: SEARCH }).fill('nothing-matches-this-at-all');
  const palette = page.getByRole('dialog', { name: 'Command palette' });
  await expect(palette.getByRole('button')).toHaveCount(0, { timeout: 30_000 });
});

test('choosing a view from the palette goes there', async ({ page }) => {
  await openHome(page);
  await page.keyboard.press('ControlOrMeta+k');
  await page
    .getByRole('dialog', { name: 'Command palette' })
    .getByRole('button', { name: 'Cluster checks view' })
    .click();
  await expect(page).toHaveTitle(/^checks /, { timeout: 60_000 });
});

test('choosing an object from the palette opens it', async ({ page }) => {
  await openHome(page);
  await page.keyboard.press('ControlOrMeta+k');
  await page.getByRole('textbox', { name: SEARCH }).fill('healthy');
  const palette = page.getByRole('dialog', { name: 'Command palette' });
  const hit = palette.getByRole('button', { name: 'e2e/healthy deployment' });
  await expect(hit).toBeVisible({ timeout: 30_000 });
  await hit.click();
  await expect(page).toHaveTitle(/^healthy deployments /, { timeout: 60_000 });
});

import { expect, test } from '../harness/test';
import { openResource, selectRow } from '../harness/app';
import type { Page } from '@playwright/test';

test.describe.configure({ mode: 'serial' });

async function openTerminal(page: Page, pod: string): Promise<void> {
  await openResource(page, 'pods', 'Pod');
  await selectRow(page, pod);
  await page.getByRole('tab', { name: 'Terminal', exact: true }).click();
  await expect(page.getByRole('tabpanel', { name: 'Terminal' })).toBeVisible({ timeout: 30_000 });
}

test('the terminal panel offers a shell on the pod that is selected', async ({ page }) => {
  await openTerminal(page, 'chatty-');
  const panel = page.getByRole('tabpanel', { name: 'Terminal' });
  await expect(panel).toContainText('No shells open', { timeout: 30_000 });
  await expect(panel.getByRole('button', { name: /^Shell in chatty-/ })).toBeVisible();
});

test('opening a shell puts a live terminal on the screen', async ({ page }) => {
  await openTerminal(page, 'chatty-');
  await page.getByRole('button', { name: /^Shell in chatty-/ }).click();
  await expect(page.locator('.xterm-screen').first()).toBeVisible({ timeout: 60_000 });
});

test('what is typed into the shell runs in the container', async ({ page }) => {
  await openTerminal(page, 'chatty-');
  const open = page.getByRole('button', { name: /^Shell in chatty-/ });
  if ((await open.count()) > 0) {
    await open.click();
  }
  await expect(page.locator('.xterm-screen').first()).toBeVisible({ timeout: 60_000 });
  await page.locator('.xterm-screen').first().click();
  await page.keyboard.type('echo shell-reached-the-container');
  await page.keyboard.press('Enter');
  await expect
    .poll(() => page.locator('.xterm-rows').first().innerText(), { timeout: 60_000 })
    .toContain('shell-reached-the-container');
});

test('a container with no shell is still offered one', async ({ page }) => {
  await openTerminal(page, 'noshell');
  await expect(
    page.getByRole('tabpanel', { name: 'Terminal' }).getByRole('button', { name: 'Shell in noshell' }),
  ).toBeVisible({ timeout: 30_000 });
});

test('a shell on this machine is refused in the browser and says why', async ({ page }) => {
  await openTerminal(page, 'chatty-');
  await expect(page.getByRole('tabpanel', { name: 'Terminal' })).toContainText(
    'a shell on this machine is only available in the desktop app',
    { timeout: 30_000 },
  );
});

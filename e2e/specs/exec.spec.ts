import { expect, test } from '../harness/test';
import { openResource, selectRow } from '../harness/app';
import { kubectl } from '../harness/cluster';
import { NAMESPACE } from '../harness/paths';
import type { Page } from '@playwright/test';

test.describe.configure({ mode: 'serial' });

async function openTerminal(page: Page, pod: string): Promise<void> {
  await openResource(page, 'pods', 'Pod');
  await selectRow(page, pod);
  await page.getByRole('tab', { name: 'Terminal', exact: true }).click();
  await expect(page.getByRole('tabpanel', { name: 'Terminal' })).toBeVisible({ timeout: 30_000 });
}

test('the terminal panel offers a shell on the pod that is selected', async ({ page }) => {
  await openTerminal(page, 'shellable');
  const panel = page.getByRole('tabpanel', { name: 'Terminal' });
  await expect(panel).toContainText('No shells open', { timeout: 30_000 });
  await expect(panel.getByRole('button', { name: 'Shell in shellable' })).toBeVisible();
});

test('opening a shell puts a live terminal on the screen', async ({ page }) => {
  await openTerminal(page, 'shellable');
  await page.getByRole('button', { name: 'Shell in shellable' }).click();
  await expect(page.locator('.xterm-screen').first()).toBeVisible({ timeout: 60_000 });
});

test('what is typed into the shell runs in the container', async ({ page }) => {
  await openTerminal(page, 'shellable');
  const open = page.getByRole('button', { name: 'Shell in shellable' });
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

test('a container with no shell is told why, not just refused', async ({ page }) => {
  await openTerminal(page, 'noshell');
  await page.getByRole('button', { name: 'Shell in noshell' }).click();
  const panel = page.getByRole('tabpanel', { name: 'Terminal' });
  await expect(panel).toContainText('has no shell, so it cannot be exec', { timeout: 60_000 });
  await expect(panel).toContainText('Kubernetes can add a temporary container beside it');
  await expect(panel.getByRole('button', { name: 'Attach debug container' })).toBeEnabled();
});

test('attaching a debug container reaches the pod spec, and gives a terminal', async ({ page }) => {
  test.setTimeout(240_000);
  await openTerminal(page, 'noshell');
  await page.getByRole('button', { name: 'Shell in noshell' }).click();
  const attach = page.getByRole('button', { name: 'Attach debug container' });
  await expect(attach).toBeEnabled({ timeout: 60_000 });
  await attach.click();

  await expect
    .poll(
      () =>
        kubectl([
          '-n',
          NAMESPACE,
          'get',
          'pod/noshell',
          '-o',
          'jsonpath={.spec.ephemeralContainers[*].name}',
        ]).trim(),
      { timeout: 150_000 },
    )
    .not.toBe('');
  await expect(page.locator('.xterm-screen').first()).toBeVisible({ timeout: 60_000 });
});

test('a root shell on a node is offered because the flag allowed it', async ({ page }) => {
  test.setTimeout(240_000);
  await openResource(page, 'nodes', 'Node');
  const node = kubectl([
    'get',
    'nodes',
    '-l',
    'spinoza.test/pool=drain',
    '-o',
    'jsonpath={.items[0].metadata.name}',
  ]).trim();
  await selectRow(page, node);
  await page.getByRole('tab', { name: 'Overview', exact: true }).click();
  const open = page.getByRole('button', { name: 'Node shell', exact: true });
  await expect(open).toBeEnabled({ timeout: 30_000 });
  await open.click();
  await expect(page.locator('.xterm-screen').first()).toBeVisible({ timeout: 120_000 });
  await expect
    .poll(
      () =>
        kubectl([
          '-n',
          'kube-system',
          'get',
          'pods',
          '-o',
          'jsonpath={.items[?(@.spec.nodeName=="' + node + '")].metadata.name}',
        ]),
      { timeout: 60_000 },
    )
    .toContain('spinoza-node-shell-');
});

test('a shell on this machine is refused in the browser and says why', async ({ page }) => {
  await openTerminal(page, 'shellable');
  await expect(page.getByRole('tabpanel', { name: 'Terminal' })).toContainText(
    'a shell on this machine is only available in the desktop app',
    { timeout: 30_000 },
  );
});

test('the shell learns its new size when the window changes', async ({ page }) => {
  test.setTimeout(240_000);
  await openTerminal(page, 'shellable');
  await page.getByRole('button', { name: 'Shell in shellable' }).click();
  const screen = page.locator('.xterm-screen').first();
  await expect(screen).toBeVisible({ timeout: 60_000 });

  async function askSize(): Promise<string> {
    await screen.click();
    await page.keyboard.type('stty size');
    await page.keyboard.press('Enter');
    let seen = '';
    await expect
      .poll(
        async () => {
          const rows = await page.locator('.xterm-rows').first().innerText();
          const sizes = rows.split('\n').filter((line) => /^\s*\d+ \d+\s*$/.test(line));
          seen = sizes.at(-1)?.trim() ?? '';
          return seen;
        },
        { timeout: 60_000 },
      )
      .not.toBe('');
    return seen;
  }

  const before = await askSize();
  expect(before).toMatch(/^\d+ \d+$/);

  await page.setViewportSize({ width: 820, height: 620 });
  await expect
    .poll(
      async () => {
        await screen.click();
        await page.keyboard.type('stty size');
        await page.keyboard.press('Enter');
        const rows = await page.locator('.xterm-rows').first().innerText();
        const sizes = rows.split('\n').filter((line) => /^\s*\d+ \d+\s*$/.test(line));
        return sizes.at(-1)?.trim() ?? '';
      },
      { timeout: 120_000 },
    )
    .not.toBe(before);
});

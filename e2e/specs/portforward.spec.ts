import { expect, test } from '../harness/test';
import { openResource } from '../harness/app';

async function openHealthy(page: import('@playwright/test').Page): Promise<void> {
  await openResource(page, 'pods', 'Pod');
  const row = page
    .locator('main tbody tr')
    .filter({ hasText: 'healthy-' })
    .filter({ hasText: 'Running' })
    .first();
  await expect(row).toBeVisible({ timeout: 90_000 });
  await row.getByRole('button').first().click();
  await page.getByRole('tab', { name: 'Overview' }).click();
}

async function clearForwards(page: import('@playwright/test').Page): Promise<void> {
  await page.evaluate(async () => {
    const response = await fetch('/api/portforward');
    const forwards = (await response.json()) as { id: string }[];
    for (const forward of forwards) {
      await fetch(`/api/portforward?id=${encodeURIComponent(forward.id)}`, { method: 'DELETE' });
    }
  });
  await page.reload();
  await page.waitForLoadState('domcontentloaded');
  await page.getByRole('tab', { name: 'Overview' }).click();
}

test('a pod with a port offers to forward it', async ({ page }) => {
  await openHealthy(page);
  await expect(page.getByText('PORTS')).toBeVisible({ timeout: 30_000 });
  await expect(page.getByText('8080').first()).toBeVisible();
  await expect(page.getByRole('button', { name: 'Forward', exact: true })).toBeVisible();
});

test('a pod with no port is not offered one', async ({ page }) => {
  await openResource(page, 'pods', 'Pod');
  const row = page.locator('main tbody tr').filter({ hasText: 'noshell' }).first();
  await expect(row).toBeVisible({ timeout: 60_000 });
  await row.getByRole('button').first().click();
  await page.getByRole('tab', { name: 'Overview' }).click();
  await expect(page.getByRole('button', { name: 'Forward', exact: true })).toHaveCount(0);
});

test('forwarding reports the local address it took', async ({ page }) => {
  await openHealthy(page);
  await clearForwards(page);
  await page.getByRole('button', { name: 'Forward', exact: true }).click();
  await page.getByRole('tab', { name: 'Forwards', exact: true }).click();
  const forwards = page.getByRole('tabpanel', { name: 'Forwards' });
  await expect(forwards).toContainText('healthy-', { timeout: 60_000 });
  await expect(forwards).toContainText(/127\.0\.0\.1:\d+|localhost:\d+|:\d{4,5}/);
  await expect(forwards.getByRole('button', { name: 'Stop', exact: true }).first()).toBeVisible({
    timeout: 60_000,
  });
  await forwards.getByRole('button', { name: 'Stop', exact: true }).first().click();
});

test('a running port is offered as open or stop, never as a duplicate forward', async ({
  page,
}) => {
  await openHealthy(page);
  await clearForwards(page);
  await page.getByRole('button', { name: 'Forward', exact: true }).click();
  await expect(page.getByRole('button', { name: /^Open 127\.0\.0\.1:/ })).toBeVisible({
    timeout: 60_000,
  });
  await expect(page.getByRole('button', { name: 'Stop forwarding port 8080' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Forward', exact: true })).toHaveCount(0);
  await page.getByRole('button', { name: 'Stop forwarding port 8080' }).click();
  await expect(page.getByRole('button', { name: 'Forward', exact: true })).toBeVisible({
    timeout: 30_000,
  });
});

test('a forward survives navigating away and back', async ({ page }) => {
  await openHealthy(page);
  await clearForwards(page);
  await page.getByRole('button', { name: 'Forward', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Stop forwarding port 8080' })).toBeVisible({
    timeout: 60_000,
  });
  await openResource(page, 'configmaps', 'ConfigMap');
  await openHealthy(page);
  await page.getByRole('tab', { name: 'Forwards', exact: true }).click();
  const forwards = page.getByRole('tabpanel', { name: 'Forwards' });
  await expect(forwards.getByRole('button', { name: 'Stop', exact: true }).first()).toBeVisible({
    timeout: 30_000,
  });
  await forwards.getByRole('button', { name: 'Stop', exact: true }).first().click();
  await expect(forwards).toContainText('No active forwards', { timeout: 30_000 });
});

test('a running forward carries real traffic to the selected pod', async ({ page }) => {
  await openHealthy(page);
  await clearForwards(page);
  await page.getByRole('button', { name: 'Forward', exact: true }).click();
  await page.getByRole('tab', { name: 'Forwards', exact: true }).click();
  const forwards = page.getByRole('tabpanel', { name: 'Forwards' });
  const link = forwards.getByRole('link', { name: /127\.0\.0\.1:/ }).first();
  await expect(link).toBeVisible({ timeout: 60_000 });
  const href = await link.getAttribute('href');
  expect(href).not.toBeNull();
  if (href === null) {
    throw new Error('the forward has no URL');
  }
  await expect
    .poll(
      async () => {
        const response = await page.request.get(href);
        if (!response.ok()) {
          return '';
        }
        return response.text();
      },
      { timeout: 60_000 },
    )
    .toContain('e2e');
  await forwards.getByRole('button', { name: 'Stop', exact: true }).first().click();
  await expect(forwards).toContainText('No active forwards', { timeout: 30_000 });
});

test('stopping a forward closes the local listener it owned', async ({ page }) => {
  await openHealthy(page);
  await clearForwards(page);
  await page.getByRole('button', { name: 'Forward', exact: true }).click();
  await page.getByRole('tab', { name: 'Forwards', exact: true }).click();
  const forwards = page.getByRole('tabpanel', { name: 'Forwards' });
  const link = forwards.getByRole('link', { name: /127\.0\.0\.1:/ }).first();
  await expect(link).toBeVisible({ timeout: 60_000 });
  const href = await link.getAttribute('href');
  expect(href).not.toBeNull();
  if (href === null) {
    throw new Error('the forward has no URL');
  }
  await expect
    .poll(async () => (await page.request.get(href)).ok(), { timeout: 60_000 })
    .toBe(true);
  await forwards.getByRole('button', { name: 'Stop', exact: true }).first().click();
  await expect(forwards).toContainText('No active forwards', { timeout: 30_000 });
  await expect
    .poll(
      async () => {
        try {
          return (await page.request.get(href, { timeout: 2_000 })).ok();
        } catch {
          return false;
        }
      },
      { timeout: 30_000 },
    )
    .toBe(false);
});

test('forward notifications can be filtered, followed, and cleared', async ({ page }) => {
  await openHealthy(page);
  await clearForwards(page);
  const notifications = page.getByLabel('Notifications', { exact: true });
  const menu = notifications.locator('..');
  await notifications.click();
  const clear = menu.getByRole('button', { name: 'Clear', exact: true });
  if (await clear.isEnabled()) {
    await clear.click();
  }
  await notifications.click();
  await page.getByRole('button', { name: 'Forward', exact: true }).click();
  await notifications.click();
  const message = menu.getByText(/Forwarding healthy-.*127\.0\.0\.1:\d+ to 8080/);
  await expect(message).toBeVisible({ timeout: 60_000 });
  const done = menu.getByRole('button', { name: 'Done', exact: true });
  await done.click();
  await expect(done).toHaveAttribute('aria-pressed', 'true');
  await expect(message).toBeVisible();
  const failures = menu.getByRole('button', { name: 'Failures', exact: true });
  await failures.click();
  await expect(menu.getByText('Nothing yet.', { exact: true })).toBeVisible();
  await menu.getByRole('button', { name: 'All', exact: true }).click();
  await menu.getByRole('button', { name: /pods\/e2e\/healthy-/ }).click();
  await expect(menu).not.toHaveAttribute('open');
  expect(page.url()).toContain('name=healthy-');
  await page.getByRole('tab', { name: 'Overview', exact: true }).click();
  await page.getByRole('button', { name: 'Stop forwarding port 8080' }).click();
  await notifications.click();
  await expect(menu.getByText(/Stopped forwarding healthy-.* port 8080/)).toBeVisible({
    timeout: 60_000,
  });
  await clear.click();
  await expect(menu.getByText('Nothing yet.', { exact: true })).toBeVisible();
  await expect(clear).toBeDisabled();
});

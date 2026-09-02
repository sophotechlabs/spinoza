import { expect, test } from '../harness/test';
import { openHome, openResource } from '../harness/app';

test('the app reports the health of its own feed', async ({ page }) => {
  await openHome(page);
  await expect(page.getByRole('status', { name: 'The cluster feed is connected' })).toBeVisible({
    timeout: 30_000,
  });
});

test('reconnecting is always offered, not only after a failure', async ({ page }) => {
  await openHome(page);
  await expect(page.getByRole('button', { name: 'Reconnect' })).toBeVisible();
});

test('a reconnect leaves the feed connected', async ({ page }) => {
  await openHome(page);
  const frames: string[] = [];
  const opened = page.waitForEvent('websocket', (socket) => {
    if (new URL(socket.url()).pathname !== '/ws') {
      return false;
    }
    socket.on('framereceived', (frame) => {
      frames.push(String(frame.payload));
    });
    return true;
  });
  await page.getByRole('button', { name: 'Reconnect' }).click();
  const socket = await opened;
  expect(new URL(socket.url()).searchParams.get('view')).toBe('browser');
  await expect
    .poll(
      () => {
        return frames.some((payload) => {
          return payload.includes('"type":"cluster"') || payload.includes('"type":"context"');
        });
      },
      { timeout: 60_000 },
    )
    .toBe(true);
  await expect(page.getByRole('status', { name: 'The cluster feed is connected' })).toBeVisible({
    timeout: 60_000,
  });
});

test('reconnecting resubscribes the resource table that is already open', async ({ page }) => {
  await openResource(page, 'pods', 'Pod');
  const healthy = page.locator('main tbody tr').filter({ hasText: 'healthy-' }).first();
  await expect(healthy).toBeVisible({ timeout: 60_000 });
  const frames: string[] = [];
  const opened = page.waitForEvent('websocket', (socket) => {
    if (new URL(socket.url()).pathname !== '/ws') {
      return false;
    }
    socket.on('framesent', (frame) => {
      frames.push(String(frame.payload));
    });
    return true;
  });
  await page.getByRole('button', { name: 'Reconnect' }).click();
  await opened;
  await expect
    .poll(
      () => {
        return frames.some((payload) => {
          return payload.includes('"type":"subscribe"') && payload.includes('"resource":"pods"');
        });
      },
      { timeout: 60_000 },
    )
    .toBe(true);
  await expect(page.getByRole('status', { name: 'The cluster feed is connected' })).toBeVisible({
    timeout: 60_000,
  });
  await expect(healthy).toBeVisible();
});

test('an integration that is absent is named and disabled, not hidden', async ({ page }) => {
  await openHome(page);
  for (const absent of ['Traffic', 'Flux', 'Argo CD']) {
    const button = page.getByRole('button', { name: absent, exact: true });
    await expect(button).toBeVisible();
    await expect(button).toBeDisabled();
  }
});

test('a metric with no source is not invented', async ({ page }) => {
  await openHome(page);
  await expect(page.getByRole('heading', { name: 'Allocatable capacity' })).toBeVisible({
    timeout: 60_000,
  });
});

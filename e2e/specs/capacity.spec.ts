import { expect, test } from '../harness/test';
import { state } from '../harness/test';
import { openHome } from '../harness/app';
import { CONTEXT } from '../harness/paths';

interface SocketResult {
  errors: string[];
  snapshots: number;
}

test('one live connection accepts 64 subscriptions and refuses the 65th', async ({ page }) => {
  await openHome(page);
  const result = await page.evaluate(async (): Promise<SocketResult> => {
    let protocol = 'ws:';
    if (location.protocol === 'https:') {
      protocol = 'wss:';
    }
    const socket = new WebSocket(`${protocol}//${location.host}/ws?view=browser`);
    await new Promise<void>((resolve, reject) => {
      socket.addEventListener('open', () => {
        resolve();
      });
      socket.addEventListener('error', () => {
        reject(new Error('the capacity socket did not open'));
      });
    });
    const errors: string[] = [];
    let snapshots = 0;
    socket.addEventListener('message', (event) => {
      const message = JSON.parse(String(event.data)) as { type?: string; message?: string };
      if (message.type === 'snapshot') {
        snapshots += 1;
      }
      if (message.type === 'error' && message.message !== undefined) {
        errors.push(message.message);
      }
    });
    for (let index = 0; index < 65; index += 1) {
      socket.send(
        JSON.stringify({
          type: 'subscribe',
          subId: `capacity-${String(index)}`,
          group: '',
          version: 'v1',
          resource: 'configmaps',
          namespace: 'e2e',
          limit: 1,
          filters: [],
        }),
      );
    }
    const deadline = Date.now() + 90_000;
    while ((errors.length === 0 || snapshots < 64) && Date.now() < deadline) {
      await new Promise<void>((resolve) => {
        setTimeout(resolve, 100);
      });
    }
    socket.close();
    return { errors, snapshots };
  });
  expect(result.snapshots).toBe(64);
  expect(result.errors).toContain('this connection already holds the maximum number of subscriptions');
});

test('repeated live connection churn leaves the server responsive', async ({ page }) => {
  await openHome(page);
  const before = await page.evaluate(async () => {
    const response = await fetch('/api/memory');
    return response.json() as Promise<{ heapMi: number; sysMi: number }>;
  });
  await page.evaluate(async () => {
    let protocol = 'ws:';
    if (location.protocol === 'https:') {
      protocol = 'wss:';
    }
    for (let cycle = 0; cycle < 20; cycle += 1) {
      const sockets = Array.from({ length: 6 }, () => {
        return new WebSocket(`${protocol}//${location.host}/ws?view=browser`);
      });
      await Promise.all(
        sockets.map(
          (socket) =>
            new Promise<void>((resolve, reject) => {
              socket.addEventListener('open', () => {
                resolve();
              });
              socket.addEventListener('error', () => {
                reject(new Error('a churn socket did not open'));
              });
            }),
        ),
      );
      await Promise.all(
        sockets.map(
          (socket) =>
            new Promise<void>((resolve) => {
              socket.addEventListener('close', () => {
                resolve();
              });
              socket.close(1000, 'churn cycle complete');
            }),
        ),
      );
    }
  });
  await expect
    .poll(
      async () => {
        const response = await page.request.get('/healthz');
        return response.status();
      },
      { timeout: 60_000 },
    )
    .toBe(200);
  const after = await page.evaluate(async () => {
    const response = await fetch('/api/memory');
    return response.json() as Promise<{ heapMi: number; sysMi: number }>;
  });
  expect(after.heapMi).toBeLessThan(before.heapMi * 2 + 64);
  expect(after.sysMi).toBeGreaterThan(0);
});

test('the scale fixture converges without rendering every row', async ({ page }) => {
  const token = encodeURIComponent(state().token);
  const hash = `#context=${CONTEXT}&version=v1&resource=configmaps&kind=ConfigMap`;
  await page.goto(`/?token=${token}${hash}`);
  await expect(page.locator('main tbody tr').first()).toBeVisible({ timeout: 120_000 });
  const rendered = await page.locator('main tbody tr').count();
  expect(rendered).toBeLessThan(500);
  await expect(page.getByText(/bulk-1499/)).toHaveCount(0);
  await page.getByPlaceholder(/filter/i).first().fill('bulk-1499');
  await expect(page.getByText('bulk-1499', { exact: true })).toBeVisible({ timeout: 120_000 });
});

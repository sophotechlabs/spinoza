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
    return new Promise<SocketResult>((resolve, reject) => {
      const socket = new WebSocket(`${protocol}//${location.host}/ws?view=browser`);
      const errors: string[] = [];
      let snapshots = 0;
      const timer = window.setTimeout(() => {
        socket.close();
        reject(
          new Error(
            `the capacity subscription did not complete: ${String(snapshots)} snapshots, ${errors.join(', ')}`,
          ),
        );
      }, 60_000);
      const subscribe = (index: number) => {
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
      };
      socket.addEventListener('open', () => {
        subscribe(0);
      });
      socket.addEventListener('error', () => {
        window.clearTimeout(timer);
        socket.close();
        reject(new Error('the capacity socket did not open'));
      });
      socket.addEventListener('message', (event) => {
        const message = JSON.parse(String(event.data)) as {
          type?: string;
          subId?: string;
          message?: string;
        };
        if (message.type === 'snapshot') {
          snapshots += 1;
          if (snapshots < 64) {
            subscribe(snapshots);
            return;
          }
          subscribe(64);
          return;
        }
        if (message.type !== 'error' || message.message === undefined) {
          return;
        }
        errors.push(message.message);
        if (snapshots < 64) {
          window.clearTimeout(timer);
          socket.close();
          reject(
            new Error(
              `${message.subId ?? 'a subscription'} was refused after ${String(snapshots)} snapshots: ${message.message}`,
            ),
          );
          return;
        }
        window.clearTimeout(timer);
        socket.close();
        resolve({ errors, snapshots });
      });
    });
  });
  expect(result.snapshots).toBe(64);
  expect(result.errors).toContain(
    'this connection already holds the maximum number of subscriptions',
  );
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
  await page
    .getByPlaceholder(/filter/i)
    .first()
    .fill('bulk-1499');
  await expect(page.getByText('bulk-1499', { exact: true })).toBeVisible({ timeout: 120_000 });
});

test('a failed subscription id can be reused for a valid live feed', async ({ page }) => {
  await openHome(page);
  const result = await page.evaluate(async () => {
    let protocol = 'ws:';
    if (location.protocol === 'https:') {
      protocol = 'wss:';
    }
    return new Promise<{ failed: string; recovered: boolean }>((resolve, reject) => {
      const socket = new WebSocket(`${protocol}//${location.host}/ws?view=browser`);
      const timer = window.setTimeout(() => {
        socket.close();
        reject(new Error('the recovery subscription did not complete'));
      }, 60_000);
      let failed = '';
      socket.addEventListener('open', () => {
        socket.send(
          JSON.stringify({
            type: 'subscribe',
            subId: 'recovery',
            group: 'spinoza.test',
            version: 'v1',
            resource: 'missing',
            namespace: 'e2e',
            limit: 1,
            filters: [],
          }),
        );
      });
      socket.addEventListener('error', () => {
        window.clearTimeout(timer);
        reject(new Error('the recovery socket did not open'));
      });
      socket.addEventListener('message', (event) => {
        const message = JSON.parse(String(event.data)) as {
          type?: string;
          subId?: string;
          message?: string;
        };
        if (message.subId !== 'recovery') {
          return;
        }
        if (message.type === 'error' && failed === '') {
          if (message.message === undefined) {
            return;
          }
          failed = message.message;
          socket.send(
            JSON.stringify({
              type: 'subscribe',
              subId: 'recovery',
              group: '',
              version: 'v1',
              resource: 'configmaps',
              namespace: 'e2e',
              limit: 1,
              filters: [],
            }),
          );
          return;
        }
        if (message.type !== 'snapshot' || failed === '') {
          return;
        }
        window.clearTimeout(timer);
        socket.close();
        resolve({ failed, recovered: true });
      });
    });
  });
  expect(result.failed).not.toBe('');
  expect(result.recovered).toBe(true);
});

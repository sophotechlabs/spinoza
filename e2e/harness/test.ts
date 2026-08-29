import { test as base } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { BASE_URL, STATE_FILE, STORAGE_STATE } from './paths';

interface State {
  pid: number;
  baseURL: string;
  token: string;
}

export function state(): State {
  return JSON.parse(readFileSync(STATE_FILE, 'utf8')) as State;
}

export function authed(hash: string): string {
  return `${BASE_URL}/?token=${state().token}${hash}`;
}

export const test = base.extend<Record<string, never>, { view: void }>({
  view: [
    async ({ browser }, use) => {
      const context = await browser.newContext({ storageState: STORAGE_STATE });
      const page = await context.newPage();
      await page.goto(
        authed('#context=kind-spinoza-e2e&version=v1&resource=pods&kind=Pod&namespace=e2e&name=noshell'),
      );
      await page.waitForLoadState('domcontentloaded');
      const proof = await page.evaluate(async () => {
        const response = await fetch('/api/version');
        return response.status;
      });
      if (proof !== 200) {
        throw new Error(`the keep-alive view is not authenticated: /api/cluster answered ${String(proof)}`);
      }
      await page
        .getByRole('tab', { name: 'Overview' })
        .first()
        .waitFor({ state: 'visible', timeout: 60_000 })
        .catch(() => undefined);
      await use();
      await context.close();
    },
    { scope: 'worker', auto: true },
  ],
});

export { expect } from '@playwright/test';

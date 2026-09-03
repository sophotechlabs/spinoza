import type { Page } from '@playwright/test';
import { CONTEXT } from './paths';
import { authed } from './test';

export function viewHash(view: string): string {
  return `#context=${CONTEXT}&view=${view}`;
}

export function resourceHash(resource: string, kind: string, version = 'v1'): string {
  return `#context=${CONTEXT}&version=${version}&resource=${resource}&kind=${kind}`;
}

async function settle(page: Page, wanted: string): Promise<void> {
  await page.waitForFunction((prefix) => document.title.startsWith(prefix), wanted, {
    timeout: 60_000,
  });
}

export async function openView(page: Page, view: string): Promise<void> {
  await page.goto(authed(viewHash(view)));
  await settle(page, view);
}

export async function openResource(
  page: Page,
  resource: string,
  kind: string,
  version = 'v1',
): Promise<void> {
  await page.goto(authed(resourceHash(resource, kind, version)));
  await settle(page, resource);
}

export async function openGrouped(
  page: Page,
  group: string,
  resource: string,
  kind: string,
  version = 'v1',
): Promise<void> {
  await page.goto(
    authed(
      `#context=${CONTEXT}&group=${group}&version=${version}&resource=${resource}&kind=${kind}`,
    ),
  );
  await settle(page, resource);
}

export async function selectRow(page: Page, name: string): Promise<void> {
  const row = page.locator('main tbody tr').filter({ hasText: name }).first();
  await row.waitFor({ state: 'visible', timeout: 60_000 });
  await row.getByRole('button').first().click();
  await page
    .getByRole('tablist', { name: 'right panels' })
    .waitFor({ state: 'visible', timeout: 60_000 });
}

export async function openHome(page: Page): Promise<void> {
  await page.goto(authed(''));
  await page.waitForLoadState('domcontentloaded');
}

export async function openPalette(page: Page): Promise<void> {
  await page.getByRole('button', { name: /^Search / }).click();
}

export async function expandCategory(page: Page, name: string): Promise<void> {
  const button = page.getByRole('button', { name: new RegExp(`^${name} \\d+$`) });
  await button.waitFor({ state: 'visible', timeout: 60_000 });
  const expanded = await button.getAttribute('aria-expanded');
  if (expanded === 'true') {
    return;
  }
  await button.click();
}

export async function ensureDrawer(page: Page): Promise<void> {
  const show = page.getByRole('button', { name: 'Show the right dock' });
  await show.click({ timeout: 2_000 }).catch(() => undefined);
}

export function sidebar(page: Page, label: string | RegExp) {
  if (typeof label === 'string') {
    return page.getByRole('button', { name: label, exact: true });
  }
  return page.getByRole('button', { name: label });
}

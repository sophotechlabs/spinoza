import { expect, test } from '../harness/test';
import { openHome } from '../harness/app';
import { BASE_URL, CONTEXT, SECOND_CONTEXT, SECOND_KUBECONFIG } from '../harness/paths';
import { state } from '../harness/test';
import type { Page } from '@playwright/test';

test.describe.configure({ mode: 'serial' });

const VIEWS = [
  'resources',
  'cluster',
  'issues',
  'topology',
  'helm',
  'checks',
  'history',
  'gitops',
  'flux-list',
  'flux-roles',
  'argo-apps',
  'argo-graph',
  'argo-list',
  'traffic',
  'fleet',
  'rbac',
];

interface Opened {
  id: string;
  context: string;
  active: boolean;
}

async function opened(page: Page): Promise<Opened[]> {
  return page.evaluate(async () => {
    const response = await fetch('/api/clusters');
    const body = (await response.json()) as { clusters: unknown[] };
    return body.clusters as never;
  });
}

async function openSecond(page: Page): Promise<number> {
  return page.evaluate(
    async ([path, name]) => {
      await fetch(`/api/kubeconfigs?path=${encodeURIComponent(path)}`, { method: 'POST' });
      const query = `name=${encodeURIComponent(name)}&kubeconfig=${encodeURIComponent(path)}`;
      const response = await fetch(`/api/clusters?${query}`, { method: 'POST' });
      return response.status;
    },
    [SECOND_KUBECONFIG, SECOND_CONTEXT],
  );
}

async function activate(page: Page, id: string): Promise<void> {
  await page.evaluate(async (cluster) => {
    await fetch(`/api/clusters/active?cluster=${encodeURIComponent(cluster)}`, { method: 'POST' });
  }, id);
}

function idOf(clusters: Opened[], context: string): string {
  const found = clusters.find((one) => one.context === context);
  if (found === undefined) {
    throw new Error(`no open cluster for ${context}`);
  }
  return found.id;
}

function url(context: string, view: string): string {
  return `${BASE_URL}/?token=${state().token}#context=${encodeURIComponent(context)}&view=${view}`;
}

async function mainText(page: Page): Promise<string> {
  return page.evaluate(() => {
    const region = document.querySelector('main');
    if (region === null) {
      return '';
    }
    return (region as HTMLElement).innerText;
  });
}

async function walk(page: Page, context: string, tag: string): Promise<void> {
  for (const view of VIEWS) {
    await page.goto(url(context, view));
    await page.waitForTimeout(3500);
    const text = await mainText(page);
    console.log(`##${tag} VIEW ${view}\n${text.slice(0, 1800)}\n##END`);
  }
}

async function sidebarReport(page: Page, tag: string): Promise<void> {
  const nav = await page.evaluate(() => {
    const aside = document.querySelector('nav');
    if (aside === null) {
      return '';
    }
    return (aside as HTMLElement).innerText;
  });
  console.log(`##${tag} SIDEBAR\n${nav.slice(0, 1500)}\n##END`);
  const buttons = await page.$$eval('button', (list) =>
    list
      .map((one) => ({
        text: (one as HTMLButtonElement).innerText.replace(/\n/g, ' ').trim(),
        disabled: (one as HTMLButtonElement).disabled,
        title: one.getAttribute('title'),
      }))
      .filter((one) => one.text !== ''),
  );
  console.log(`##${tag} BUTTONS ${JSON.stringify(buttons)}\n##END`);
}

test('design: the seeded cluster, every view', async ({ page }) => {
  await openHome(page);
  await sidebarReport(page, 'SEEDED');
  await walk(page, CONTEXT, 'SEEDED');
});

test('design: a bare cluster, every view', async ({ page }) => {
  await openHome(page);
  expect(await openSecond(page)).toBe(200);
  await expect.poll(async () => (await opened(page)).length, { timeout: 90_000 }).toBe(2);
  await activate(page, idOf(await opened(page), SECOND_CONTEXT));
  await expect
    .poll(async () => (await opened(page)).find((one) => one.active)?.context, { timeout: 90_000 })
    .toBe(SECOND_CONTEXT);
  await page.goto(url(SECOND_CONTEXT, 'cluster'));
  await page.waitForTimeout(6000);
  await sidebarReport(page, 'BARE');
  await walk(page, SECOND_CONTEXT, 'BARE');
});

test('design: the fleet view with two clusters open', async ({ page }) => {
  await openHome(page);
  await activate(page, idOf(await opened(page), CONTEXT));
  await page.goto(url(CONTEXT, 'fleet'));
  await page.waitForTimeout(9000);
  const tabs = await page.$$eval('main button', (list) =>
    list.map((one) => (one as HTMLButtonElement).innerText.trim()).filter((one) => one !== ''),
  );
  console.log(`##FLEET TABS ${JSON.stringify(tabs)}\n##END`);
  for (const tab of ['Clusters', 'What is on them', 'Releases', 'Delivery', 'Images']) {
    const button = page.getByRole('button', { name: tab, exact: true });
    const count = await button.count();
    if (count > 0) {
      await button.first().click();
      await page.waitForTimeout(3000);
    }
    const rows = await page.$$eval('main table tr', (list) =>
      list.slice(0, 12).map((row) => [...(row as HTMLTableRowElement).cells].map((cell) => cell.innerText.replace(/\n/g, '|'))),
    );
    const text = await mainText(page);
    console.log(`##FLEET ${tab} rows=${JSON.stringify(rows)}\ntext=${text.slice(0, 900)}\n##END`);
  }
});

test('design: metrics on a cluster with no metrics API', async ({ page }) => {
  await openHome(page);
  await activate(page, idOf(await opened(page), SECOND_CONTEXT));
  await page.goto(`${BASE_URL}/?token=${state().token}#context=${encodeURIComponent(SECOND_CONTEXT)}&version=v1&resource=pods&kind=Pod`);
  await page.waitForTimeout(6000);
  const row = page.locator('main tbody tr').first();
  await row.click({ timeout: 60_000 });
  await page.waitForTimeout(2500);
  const metrics = page.getByRole('tab', { name: 'Metrics' });
  if ((await metrics.count()) > 0) {
    await metrics.first().click();
    await page.waitForTimeout(20_000);
  }
  const text = await page.evaluate(() => document.body.innerText);
  const lines = text.split('\n').filter((one) => /measur|metric|Metrics|not available|unavailable/i.test(one));
  console.log(`##BARE METRICS ${JSON.stringify(lines.slice(0, 14))}\n##END`);
});

async function ensureSecond(page: Page): Promise<void> {
  const already = (await opened(page)).some((one) => one.context === SECOND_CONTEXT);
  if (already) {
    return;
  }
  await openSecond(page);
  await expect.poll(async () => (await opened(page)).length, { timeout: 90_000 }).toBe(2);
}

test('design: switching clusters while a Flux view is open', async ({ page }) => {
  await openHome(page);
  await ensureSecond(page);
  await activate(page, idOf(await opened(page), CONTEXT));
  await page.goto(url(CONTEXT, 'flux-roles'));
  await page.waitForTimeout(5000);
  console.log(`##SWITCH BEFORE\n${(await mainText(page)).slice(0, 400)}\n##END`);
  const tab = page.getByRole('button', { name: SECOND_CONTEXT, exact: true });
  console.log(`##SWITCH TAB count=${String(await tab.count())}`);
  await tab.first().click();
  await page.waitForTimeout(12_000);
  console.log(`##SWITCH AFTER hash=${await page.evaluate(() => location.hash)}\n${(await mainText(page)).slice(0, 700)}\n##END`);
  const flux = page.getByRole('button', { name: /FLUX/ });
  if ((await flux.count()) > 0) {
    console.log(`##SWITCH SIDEBAR flux disabled=${String(await flux.first().isDisabled())} title=${String(await flux.first().getAttribute('title'))}\n##END`);
  }
});

test('design: the resource table at scale', async ({ page }) => {
  await openHome(page);
  await activate(page, idOf(await opened(page), CONTEXT));
  await page.waitForTimeout(2000);
  await page.goto(
    `${BASE_URL}/?token=${state().token}#context=${encodeURIComponent(CONTEXT)}&version=v1&resource=configmaps&kind=ConfigMap`,
  );
  await page.waitForTimeout(12_000);
  const rows = await page.locator('main tbody tr').count();
  const head = (await mainText(page)).split('\n').slice(0, 12);
  console.log(`##SCALE configmaps rows=${String(rows)} head=${JSON.stringify(head)}\n##END`);
  const chrome = await page.$$eval('main', (list) =>
    list
      .map((one) => (one as HTMLElement).innerText)
      .join('\n')
      .split('\n')
      .filter((line) => /of |showing|more|truncat|first |dropped|capped/i.test(line))
      .slice(0, 12),
  );
  console.log(`##SCALE configmaps counters=${JSON.stringify(chrome)}\n##END`);
  await page.goto(
    `${BASE_URL}/?token=${state().token}#context=${encodeURIComponent(CONTEXT)}&version=v1&resource=pods&kind=Pod`,
  );
  await page.waitForTimeout(12_000);
  const podRows = await page.locator('main tbody tr').count();
  const podHead = (await mainText(page)).split('\n').slice(0, 12);
  console.log(`##SCALE pods rows=${String(podRows)} head=${JSON.stringify(podHead)}\n##END`);
});

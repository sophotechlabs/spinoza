import { expect, test } from '../harness/test';
import { openView } from '../harness/app';

async function clearBaseline(page: import('@playwright/test').Page): Promise<void> {
  await page.evaluate(async () => {
    await fetch('/api/checks/baseline', { method: 'DELETE' });
  });
}

async function clearMutes(page: import('@playwright/test').Page): Promise<void> {
  const mutes = await page.evaluate(async () => {
    const response = await fetch('/api/checks/mutes');
    const body = (await response.json()) as { mutes?: unknown[] } | unknown[];
    if (Array.isArray(body)) {
      return body;
    }
    if (body.mutes === undefined) {
      return [];
    }
    return body.mutes;
  });
  for (const mute of mutes) {
    await page.evaluate(async (held) => {
      await fetch('/api/checks/mutes', {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(held),
      });
    }, mute);
  }
}

test('taking and clearing a baseline changes what the audit compares', async ({ page }) => {
  await openView(page, 'checks');
  await clearBaseline(page);
  await page.reload();
  await expect(page.getByText(/No baseline taken/)).toBeVisible({ timeout: 90_000 });
  await page.getByRole('button', { name: 'Take a baseline', exact: true }).click();
  await expect(page.getByText(/Comparing against/)).toBeVisible({ timeout: 90_000 });
  await page.getByRole('button', { name: 'forget it', exact: true }).click();
  await expect(page.getByText(/No baseline taken/)).toBeVisible({ timeout: 90_000 });
});

test('severity and namespace controls change the actual findings request', async ({ page }) => {
  await openView(page, 'checks');
  const requests: string[] = [];
  page.on('request', (request) => {
    if (request.url().includes('/api/checks?')) {
      requests.push(request.url());
    }
  });
  await page.getByLabel('Lowest severity to show').selectOption('high');
  await expect.poll(() => requests.some((url) => url.includes('severity=high'))).toBe(true);
  await page.getByLabel('Namespaces to skip').fill('kube-system,e2e-gitops');
  await expect
    .poll(() => requests.some((url) => url.includes('skip=kube-system') && url.includes('e2e-gitops')))
    .toBe(true);
});

test('a finding can be muted with a reason and unmuted again', async ({ page }) => {
  await openView(page, 'checks');
  await clearMutes(page);
  await page.reload();
  const mute = page.getByRole('button', { name: /^Mute / }).first();
  await expect(mute).toBeVisible({ timeout: 90_000 });
  const label = await mute.getAttribute('aria-label');
  expect(label).not.toBeNull();
  await mute.click();
  await page.getByLabel('Why this one is being muted').fill('e2e maintenance window');
  await page.getByRole('button', { name: 'Mute this one', exact: true }).click();
  if (label === null) {
    throw new Error('the mute button has no accessible label');
  }
  const target = label.replace(/^Mute /, '');
  const unmute = page.getByRole('button', { name: `Unmute ${target}`, exact: true });
  await expect(unmute).toBeVisible({ timeout: 90_000 });
  await unmute.click();
  await expect(page.getByRole('button', { name: label, exact: true })).toBeVisible({
    timeout: 90_000,
  });
});

test('the findings export is a nonempty CSV with the active audit columns', async ({ page }) => {
  await openView(page, 'checks');
  const download = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Export', exact: true }).click();
  const saved = await download;
  expect(saved.suggestedFilename()).toBe('spinoza-checks.csv');
  const stream = await saved.createReadStream();
  expect(stream).not.toBeNull();
  let body = '';
  if (stream !== null) {
    for await (const chunk of stream) {
      body += chunk.toString();
    }
  }
  expect(body).toContain('severity');
  expect(body.split('\n').length).toBeGreaterThan(1);
});

test('invalid personal CEL rules are diagnosed before they replace the audit', async ({ page }) => {
  await openView(page, 'checks');
  await page.getByLabel('Your own rules').fill('not valid cel {{{');
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(page.getByText(/invalid|error|parse/i).first()).toBeVisible({ timeout: 30_000 });
});

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
    const body = (await response.json()) as unknown;
    if (Array.isArray(body)) {
      return body;
    }
    if (body === null || typeof body !== 'object') {
      return [];
    }
    if (!('mutes' in body)) {
      return [];
    }
    if (!Array.isArray(body.mutes)) {
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

async function storeRules(page: import('@playwright/test').Page, rules: string): Promise<void> {
  await page.evaluate(async (held) => {
    const response = await fetch('/api/settings', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ values: { 'spinoza.checks.rules.v1': held } }),
    });
    if (!response.ok) {
      throw new Error(`saving rules returned ${String(response.status)}`);
    }
  }, rules);
}

async function readRules(page: import('@playwright/test').Page): Promise<string> {
  return page.evaluate(async () => {
    const response = await fetch('/api/settings');
    if (!response.ok) {
      throw new Error(`reading settings returned ${String(response.status)}`);
    }
    const body = (await response.json()) as { values?: Record<string, unknown> };
    const value = body.values?.['spinoza.checks.rules.v1'];
    if (typeof value === 'string') {
      return value;
    }
    return '';
  });
}

function settingsWrite(page: import('@playwright/test').Page) {
  return page.waitForResponse(
    (response) => response.url().includes('/api/settings') && response.request().method() === 'PUT',
    { timeout: 30_000 },
  );
}

async function firstMute(
  page: import('@playwright/test').Page,
): Promise<import('@playwright/test').Locator> {
  const check = page.getByRole('button', { name: /Privileged containers/ });
  await expect(check).toBeVisible({ timeout: 90_000 });
  if ((await check.getAttribute('aria-expanded')) !== 'true') {
    await check.click();
  }
  const mute = page.getByRole('button', { name: /^Mute / }).first();
  await expect(mute).toBeVisible({ timeout: 90_000 });
  return mute;
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

test('a saved baseline can be cleared and restored from its file', async ({ page }) => {
  test.setTimeout(240_000);
  await openView(page, 'checks');
  await clearBaseline(page);
  await page.reload();
  try {
    await page.getByRole('button', { name: 'Take a baseline', exact: true }).click();
    const baseline = page.getByText(/Comparing against/).first();
    await expect(baseline).toBeVisible({ timeout: 90_000 });
    const label = await baseline.innerText();
    const started = page.waitForEvent('download');
    await page.getByRole('button', { name: 'Save it to a file', exact: true }).click();
    const download = await started;
    expect(download.suggestedFilename()).toBe('spinoza-baseline.json');
    const saved = await download.path();
    expect(saved).not.toBeNull();
    if (saved === null) {
      throw new Error('the baseline download has no file');
    }
    await page.getByRole('button', { name: 'forget it', exact: true }).click();
    await expect(page.getByText(/No baseline taken/)).toBeVisible({ timeout: 90_000 });
    await page.getByLabel('A baseline to load').setInputFiles(saved);
    await expect(page.getByText(label, { exact: true })).toBeVisible({ timeout: 90_000 });
  } finally {
    await clearBaseline(page);
  }
});

test('severity and namespace controls change the actual findings request', async ({ page }) => {
  await openView(page, 'checks');
  const severity = page.getByLabel('Lowest severity to show');
  const namespaces = page.getByLabel('Namespaces to skip');
  const originalSeverity = await severity.inputValue();
  const originalNamespaces = await namespaces.inputValue();
  let changedSeverity = 'high';
  if (originalSeverity === changedSeverity) {
    changedSeverity = 'medium';
  }
  let changedNamespaces = 'kube-system,e2e-gitops';
  if (originalNamespaces === changedNamespaces) {
    changedNamespaces = 'e2e-gitops';
  }
  const requests: string[] = [];
  page.on('request', (request) => {
    if (request.url().includes('/api/checks?')) {
      requests.push(request.url());
    }
  });
  try {
    let saved = settingsWrite(page);
    await severity.selectOption(changedSeverity);
    await saved;
    await expect
      .poll(() =>
        requests.some((url) => new URL(url).searchParams.get('minSeverity') === changedSeverity),
      )
      .toBe(true);
    saved = settingsWrite(page);
    await namespaces.fill(changedNamespaces);
    await saved;
    await expect
      .poll(() =>
        requests.some(
          (url) => new URL(url).searchParams.get('skipNamespaces') === changedNamespaces,
        ),
      )
      .toBe(true);
  } finally {
    const saved = settingsWrite(page);
    await severity.selectOption(originalSeverity);
    await namespaces.fill(originalNamespaces);
    await saved;
  }
});

test('a finding can be muted with a reason and unmuted again', async ({ page }) => {
  await openView(page, 'checks');
  await clearMutes(page);
  await page.reload();
  try {
    const mute = await firstMute(page);
    const label = await mute.getAttribute('aria-label');
    expect(label).not.toBeNull();
    await mute.click();
    await page.getByLabel('Why this one is being muted').fill('e2e maintenance window');
    await page.getByRole('button', { name: 'Mute this one', exact: true }).click();
    if (label === null) {
      throw new Error('the mute button has no accessible label');
    }
    await page.getByText('What you have muted', { exact: true }).click();
    const ledger = page.getByText('What you have muted', { exact: true }).locator('..');
    await expect(ledger).toContainText('e2e maintenance window');
    const unmute = ledger.getByRole('button', { name: /^Unmute / }).first();
    await expect(unmute).toBeVisible({ timeout: 90_000 });
    await unmute.click();
    await expect(page.getByRole('button', { name: label, exact: true })).toBeVisible({
      timeout: 90_000,
    });
  } finally {
    await clearMutes(page);
  }
});

test('a mute reason survives a reload and is removable from the mute ledger', async ({ page }) => {
  await openView(page, 'checks');
  await clearMutes(page);
  await page.reload();
  try {
    const mute = await firstMute(page);
    const label = await mute.getAttribute('aria-label');
    expect(label).not.toBeNull();
    await mute.click();
    await page.getByLabel('Why this one is being muted').fill('e2e persisted reason');
    await page.getByRole('button', { name: 'Mute this one', exact: true }).click();
    await page.reload();
    await page.getByText('What you have muted', { exact: true }).click();
    const ledger = page.getByText('What you have muted', { exact: true }).locator('..');
    await expect(ledger).toContainText('e2e persisted reason', { timeout: 90_000 });
    if (label === null) {
      throw new Error('the mute button has no accessible label');
    }
    await ledger
      .getByRole('button', { name: new RegExp(`^Unmute `) })
      .first()
      .click();
    await expect(ledger).toContainText('You have not muted anything on this cluster.');
  } finally {
    await clearMutes(page);
  }
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
  await page.getByText('Your own rules', { exact: true }).click();
  await page.getByLabel('Your own rules').fill('not valid cel {{{');
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(page.getByText(/invalid|error|parse/i).first()).toBeVisible({ timeout: 30_000 });
});

test('a valid personal rule is checked, saved, and restored in a new document', async ({
  page,
}) => {
  test.setTimeout(180_000);
  const rules = JSON.stringify([
    {
      id: 'e2e-saved-rule',
      title: 'E2E saved rule',
      match: 'Deployment',
      expr: "object.metadata.name == 'healthy'",
    },
  ]);
  await openView(page, 'checks');
  const originalRules = await readRules(page);
  await storeRules(page, '');
  await page.reload();
  try {
    await page.getByText('Your own rules', { exact: true }).click();
    const editor = page.getByLabel('Your own rules');
    await editor.fill(rules);
    await page.getByRole('button', { name: 'Check', exact: true }).click();
    await expect(page.getByText('Every rule reads.', { exact: true })).toBeVisible({
      timeout: 30_000,
    });
    const saved = page.waitForResponse(
      (response) =>
        response.url().includes('/api/settings') && response.request().method() === 'PUT',
      { timeout: 30_000 },
    );
    await page.getByRole('button', { name: 'Save', exact: true }).click();
    await saved;
    await page.reload();
    await page.getByText('Your own rules', { exact: true }).click();
    await expect(page.getByLabel('Your own rules')).toHaveValue(rules);
    await expect(page.getByText('E2E saved rule', { exact: true })).toBeVisible({
      timeout: 90_000,
    });
  } finally {
    await storeRules(page, originalRules);
  }
});

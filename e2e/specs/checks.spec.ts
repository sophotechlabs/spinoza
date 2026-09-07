import { writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test } from '../harness/test';
import { openView } from '../harness/app';
import { TMP_DIR } from '../harness/paths';

async function settingsValue(page: import('@playwright/test').Page, key: string): Promise<string> {
  return page.evaluate(async (wanted) => {
    const response = await fetch('/api/settings');
    if (!response.ok) {
      throw new Error(`reading settings returned ${String(response.status)}`);
    }
    const body = (await response.json()) as { values?: Record<string, unknown> };
    const value = body.values?.[wanted];
    if (typeof value === 'string') {
      return value;
    }
    return '';
  }, key);
}

async function storeSetting(
  page: import('@playwright/test').Page,
  key: string,
  value: string,
): Promise<void> {
  await page.evaluate(
    async ([wanted, held]) => {
      const response = await fetch('/api/settings', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ values: { [wanted]: held } }),
      });
      if (!response.ok) {
        throw new Error(`restoring settings returned ${String(response.status)}`);
      }
    },
    [key, value],
  );
}

test('the scan reports findings across every family', async ({ page }) => {
  await openView(page, 'checks');
  const main = page.locator('main');
  await expect(main).toContainText(/\d+ findings across \d+ workloads/, { timeout: 60_000 });
  await expect(page.getByRole('heading', { name: 'Security' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Reliability' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Efficiency' })).toBeVisible();
});

test('the seeded workload trips the checks it was built to trip', async ({ page }) => {
  await openView(page, 'checks');
  const main = page.locator('main');
  await expect(main).toContainText('Privileged containers', { timeout: 60_000 });
  await expect(main).toContainText('Host namespaces shared');
  await expect(main).toContainText('Dangerous capabilities added');
});

test('a check nothing trips is reported clean rather than hidden', async ({ page }) => {
  await openView(page, 'checks');
  await expect(page.locator('main')).toContainText('clean', { timeout: 60_000 });
});

test('findings carry the framework that asks for them', async ({ page }) => {
  await openView(page, 'checks');
  const main = page.locator('main');
  await expect(main).toContainText('NSA/CISA', { timeout: 60_000 });
  await expect(main).toContainText('PSS');
});

test('a check opens into the affected workload, evidence, remedy, and patch', async ({ page }) => {
  await openView(page, 'checks');
  const check = page.getByRole('button', { name: /Privileged containers/ });
  await expect(check).toBeVisible({ timeout: 60_000 });
  await check.click();
  await expect(check).toHaveAttribute('aria-expanded', 'true');
  const main = page.locator('main');
  await expect(main).toContainText('Holds every capability on the node');
  await expect(main).toContainText('Drop securityContext.privileged');
  await expect(main).toContainText('risky');
  await expect(main).toContainText('privileged: false');
});

test('a check can be turned off and restored without losing the audit', async ({ page }) => {
  await openView(page, 'checks');
  const original = await settingsValue(page, 'spinoza.settings.v1');
  const check = page.getByRole('button', { name: /Privileged containers/ });
  await expect(check).toBeVisible({ timeout: 60_000 });
  await check.click();
  await expect(check).toHaveAttribute('aria-expanded', 'true');
  const turnOff = page.getByRole('button', { name: 'Turn off privileged-containers' });
  const disabled = page.waitForResponse(
    (response) => response.url().includes('/api/settings') && response.request().method() === 'PUT',
  );
  await turnOff.click();
  await disabled;
  try {
    await expect(check).toHaveCount(0, { timeout: 60_000 });
  } finally {
    await storeSetting(page, 'spinoza.settings.v1', original);
    await page.reload();
  }
  await expect(page.getByRole('button', { name: /Privileged containers/ })).toBeVisible({
    timeout: 60_000,
  });
});

test('audit scope controls change the scan request and can be restored', async ({ page }) => {
  await openView(page, 'checks');
  await expect(page.getByText('Privileged containers')).toBeVisible({ timeout: 60_000 });
  const requests: string[] = [];
  page.on('request', (request) => {
    if (request.url().includes('/api/checks?')) {
      requests.push(request.url());
    }
  });
  const wholeCluster = page.getByLabel('Audit the whole cluster');
  const everyKind = page.getByLabel('Read every kind');
  const wholeBefore = await wholeCluster.isChecked();
  const everyBefore = await everyKind.isChecked();
  const wholeChanged = !wholeBefore;
  const everyChanged = !everyBefore;
  try {
    await wholeCluster.setChecked(wholeChanged);
    await expect
      .poll(() =>
        requests.some((url) => {
          const asked = new URL(url).searchParams.get('wholeCluster');
          if (wholeChanged) {
            return asked === null;
          }
          return asked === '0';
        }),
      )
      .toBe(true);
    await everyKind.setChecked(everyChanged);
    await expect
      .poll(() =>
        requests.some((url) => {
          const asked = new URL(url).searchParams.get('everyKind');
          if (everyChanged) {
            return asked === '1';
          }
          return asked === null;
        }),
      )
      .toBe(true);
  } finally {
    const restoreWhole = (await wholeCluster.isChecked()) !== wholeBefore;
    const restoreEvery = (await everyKind.isChecked()) !== everyBefore;
    if (restoreWhole || restoreEvery) {
      const saved = page.waitForResponse(
        (response) =>
          response.url().includes('/api/settings') && response.request().method() === 'PUT',
      );
      await wholeCluster.setChecked(wholeBefore);
      await everyKind.setChecked(everyBefore);
      await saved;
    }
  }
  await expect(wholeCluster).toBeChecked({ checked: wholeBefore });
  await expect(everyKind).toBeChecked({ checked: everyBefore });
});

test('the namespace breakdown drills into one namespace and back out', async ({ page }) => {
  await openView(page, 'checks');
  const summary = page
    .locator('summary')
    .filter({ hasText: /namespaces with findings|Showing e2e only/ })
    .first();
  await expect(summary).toBeVisible({ timeout: 60_000 });
  const breakdown = summary.locator('..');
  if ((await summary.innerText()) === 'Showing e2e only') {
    if ((await breakdown.getAttribute('open')) === null) {
      await summary.click();
    }
    await breakdown.getByRole('button', { name: 'e2e', exact: true }).click();
    await expect(summary).toContainText('namespaces with findings');
  }
  await expect(summary).toContainText('namespaces with findings');
  try {
    if ((await breakdown.getAttribute('open')) === null) {
      await summary.click();
    }
    await breakdown.getByRole('button', { name: 'e2e', exact: true }).click();
    await expect(page.getByText(/findings in e2e/)).toBeVisible({ timeout: 60_000 });
    await expect(summary).toHaveText('Showing e2e only');
  } finally {
    if ((await summary.innerText()) === 'Showing e2e only') {
      if ((await breakdown.getAttribute('open')) === null) {
        await summary.click();
      }
      await breakdown.getByRole('button', { name: 'e2e', exact: true }).click();
    }
  }
  await expect(page.getByText(/findings across \d+ workloads/)).toBeVisible({ timeout: 60_000 });
});

test('a valid personal rules document is checked without replacing the audit', async ({ page }) => {
  await openView(page, 'checks');
  await page.getByText('Your own rules', { exact: true }).click();
  await page.getByLabel('Your own rules').fill('[]');
  await page.getByRole('button', { name: 'Check', exact: true }).click();
  await expect(page.getByText('Every rule reads.', { exact: true })).toBeVisible();
  await expect(page.getByText('Privileged containers')).toBeVisible({ timeout: 60_000 });
});

test('findings imported from a file land on the objects they name and say when they do not', async ({
  page,
}) => {
  const file = join(TMP_DIR, 'imported-findings.json');
  writeFileSync(
    file,
    JSON.stringify({
      tool: 'e2escan',
      findings: [
        {
          id: 'floating-tag',
          title: 'Image tag floats',
          severity: 'medium',
          category: 'security',
          wrong: 'A floating tag can change under the deployment.',
          remedy: 'Pin the image.',
          detail: 'container app runs a floating tag',
          container: 'app',
          object: { apiVersion: 'apps/v1', kind: 'Deployment', namespace: 'e2e', name: 'healthy' },
        },
        {
          id: 'floating-tag',
          detail: 'container app runs a floating tag',
          object: { apiVersion: 'apps/v1', kind: 'Deployment', namespace: 'e2e', name: 'nowhere' },
        },
      ],
    }),
  );
  await openView(page, 'checks');
  const key = 'spinoza.checks.imports.v1';
  if ((await settingsValue(page, key)) !== '') {
    await storeSetting(page, key, '');
    await openView(page, 'checks');
  }
  await page.getByText('Imported findings', { exact: true }).click();
  const editor = page.getByLabel('Imported findings');
  const save = page.getByRole('button', { name: 'Save paths', exact: true });
  try {
    await editor.fill(file);
    await save.click();
    await expect(save).toBeDisabled();

    const group = page.getByRole('button', { name: /Image tag floats/ });
    await expect(group).toBeVisible({ timeout: 60_000 });
    await expect(group).toContainText('imported-findings.json');
    await group.click();
    await expect(page.getByText('not in this cluster', { exact: true })).toBeVisible();
    await expect(
      page.getByRole('button', { name: 'Deployment · e2e/healthy · container app', exact: true }),
    ).toBeVisible();
    await expect(page.getByText('Deployment · e2e/nowhere', { exact: true })).toBeVisible();

    await editor.fill('');
    await save.click();
    await expect(page.getByRole('button', { name: /Image tag floats/ })).toHaveCount(0, {
      timeout: 60_000,
    });
  } finally {
    await storeSetting(page, key, '');
  }
});

test('a personal rule can reach the rest of the cluster and judge a Service', async ({ page }) => {
  test.setTimeout(180_000);
  const rules = JSON.stringify([
    {
      id: 'service-without-notes',
      match: 'Service',
      expr: "cluster.get('', 'configmaps', object.metadata.namespace, 'notes') == null",
    },
  ]);
  await openView(page, 'checks');
  const key = 'spinoza.checks.rules.v1';
  const original = await settingsValue(page, key);
  await storeSetting(page, key, '');
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

    const group = page.getByRole('button', { name: /service-without-notes/ });
    await expect(group).toBeVisible({ timeout: 90_000 });
    await group.click();
    await expect(
      page.getByRole('button', { name: 'Service · e2e/healthy', exact: true }),
    ).toBeVisible();
  } finally {
    await storeSetting(page, key, original);
    await page.reload();
  }
  await expect(page.getByRole('button', { name: /service-without-notes/ })).toHaveCount(0, {
    timeout: 60_000,
  });
});

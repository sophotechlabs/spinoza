import { expect, test } from '../harness/test';
import { openView } from '../harness/app';
import { kubectl } from '../harness/cluster';
import { NAMESPACE } from '../harness/paths';

test('the queue ranks a crash loop as broken and names its cause', async ({ page }) => {
  await openView(page, 'issues');
  const main = page.locator('main');
  await expect(main).toContainText('broken', { timeout: 60_000 });
  await expect(main).toContainText('crashing');
  await expect(main).toContainText('CrashLoopBackOff');
});

test('the queue says what to do about it', async ({ page }) => {
  await openView(page, 'issues');
  await expect(page.locator('main')).toContainText("read the container's logs", {
    timeout: 60_000,
  });
});

test('the queue folds the objects behind one row', async ({ page }) => {
  await openView(page, 'issues');
  await expect(page.locator('main')).toContainText(/\d+ object/, { timeout: 60_000 });
});

test('the queue clears itself once the workload stops failing', async ({ page }) => {
  const before = kubectl([
    '-n',
    NAMESPACE,
    'get',
    'deployment/crashing',
    '-o',
    'jsonpath={.spec.replicas}',
  ]).trim();
  expect(Number(before)).toBeGreaterThan(0);
  await openView(page, 'issues');
  try {
    await expect(page.locator('main')).toContainText('crashing', { timeout: 60_000 });
    kubectl(['-n', NAMESPACE, 'scale', 'deployment/crashing', '--replicas=0']);
    await expect(page.locator('main')).not.toContainText('CrashLoopBackOff', { timeout: 120_000 });
    kubectl(['-n', NAMESPACE, 'scale', 'deployment/crashing', `--replicas=${before}`]);
    await expect(page.locator('main')).toContainText('CrashLoopBackOff', { timeout: 180_000 });
  } finally {
    kubectl(['-n', NAMESPACE, 'scale', 'deployment/crashing', `--replicas=${before}`]);
  }
});

test('the queue exposes every severity tally even when one is zero', async ({ page }) => {
  await openView(page, 'issues');
  const main = page.locator('main');
  await expect(main.getByText(/\d+ broken/)).toBeVisible({ timeout: 60_000 });
  await expect(main.getByText(/\d+ degraded/)).toBeVisible();
  await expect(main.getByText(/\d+ warning/)).toBeVisible();
});

test('the issue order picker asks for newest and oldest independently', async ({ page }) => {
  await openView(page, 'issues');
  await expect(page.locator('main')).toContainText('CrashLoopBackOff', { timeout: 60_000 });
  const requests: string[] = [];
  page.on('request', (request) => {
    if (request.url().includes('/api/issues?')) {
      requests.push(request.url());
    }
  });
  const sort = page.getByRole('combobox', { name: 'Sort issues' });
  await expect(sort.getByRole('option')).toHaveText([
    'Worst first',
    'Newest first',
    'Oldest first',
  ]);
  await sort.selectOption('newest');
  await expect.poll(() => requests.some((url) => url.includes('sort=newest'))).toBe(true);
  await sort.selectOption('oldest');
  await expect.poll(() => requests.some((url) => url.includes('sort=oldest'))).toBe(true);
  await sort.selectOption('worst');
  await expect(sort).toHaveValue('worst');
});

test('a folded issue reveals its concrete objects and hides them again', async ({ page }) => {
  await openView(page, 'issues');
  const show = page.getByRole('button', { name: /^Show the .* folded under crashing$/ });
  await expect(show).toBeVisible({ timeout: 60_000 });
  const issue = page.locator('main').getByRole('button').filter({ hasText: 'crashing' }).first();
  const row = issue.locator('xpath=ancestor::li[1]');
  await show.click();
  const hide = page.getByRole('button', { name: /^Hide the .* folded under crashing$/ });
  await expect(hide).toHaveAttribute('aria-expanded', 'true');
  await expect(row.locator('ul > li').first()).toBeVisible();
  await hide.click();
  await expect(row.locator('ul > li')).toHaveCount(0);
});

test('opening an issue selects the object behind the diagnosis', async ({ page }) => {
  await openView(page, 'issues');
  const issue = page.locator('main').getByRole('button').filter({ hasText: 'crashing' }).first();
  await expect(issue).toBeVisible({ timeout: 60_000 });
  await issue.click();
  await expect(page.getByRole('tablist', { name: 'right panels' })).toBeVisible({
    timeout: 60_000,
  });
  await expect(page).toHaveTitle(/^crashing issues /);
});

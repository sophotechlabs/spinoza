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
  await openView(page, 'issues');
  await expect(page.locator('main')).toContainText('crashing', { timeout: 60_000 });
  kubectl(['-n', NAMESPACE, 'scale', 'deployment/crashing', '--replicas=0']);
  await expect(page.locator('main')).not.toContainText('CrashLoopBackOff', { timeout: 120_000 });
  kubectl(['-n', NAMESPACE, 'scale', 'deployment/crashing', '--replicas=1']);
  await expect(page.locator('main')).toContainText('CrashLoopBackOff', { timeout: 180_000 });
});

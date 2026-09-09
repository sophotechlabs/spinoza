import { expect, test } from '../harness/test';
import { openResource } from '../harness/app';
import { CLUSTER } from '../harness/paths';
import { mustRun, run } from '../harness/run';

const CONTROL_PLANE = `${CLUSTER}-control-plane`;

function stopAnswering(): void {
  mustRun('docker', ['pause', CONTROL_PLANE]);
}

function answerAgain(): void {
  run('docker', ['unpause', CONTROL_PLANE]);
}

test('a cluster that stops answering says so, keeps its rows, and is not explained twice', async ({
  page,
}) => {
  await openResource(page, 'pods', 'Pod');
  const rows = page.locator('main tbody tr');
  await expect(rows.first()).toBeVisible({ timeout: 60_000 });
  const before = await rows.count();

  stopAnswering();
  try {
    const banner = page.getByRole('status', { name: 'The cluster stopped answering' });
    await expect(banner).toBeVisible({ timeout: 60_000 });
    await expect
      .poll(async () => (await banner.textContent()) ?? '', { timeout: 120_000 })
      .toContain('stopped answering');

    expect(await rows.count()).toBe(before);
    await expect(page.locator('main').getByText(/as of \d\d:\d\d:\d\d/).first()).toBeVisible();
    await expect(page.getByText(/stopped updating/)).toHaveCount(0);
  } finally {
    answerAgain();
  }

  await expect(page.getByRole('status', { name: 'The cluster stopped answering' })).toBeHidden({
    timeout: 120_000,
  });
  await expect(page.getByText(/is answering again/)).toBeVisible({ timeout: 30_000 });
});

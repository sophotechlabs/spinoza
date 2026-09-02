import { expect, test } from '../harness/test';
import { kubectl, kubectlApply } from '../harness/cluster';
import { CONTEXT, NAMESPACE } from '../harness/paths';
import { openHome, openView, sidebar } from '../harness/app';
import { primaryShortcut } from '../harness/keyboard';
import type { Page } from '@playwright/test';

interface NodeList {
  items: {
    status?: { conditions?: { type?: string; status?: string }[] };
  }[];
}

interface PodList {
  items: {
    status?: { phase?: string };
  }[];
}

interface Version {
  serverVersion: { gitVersion: string };
}

interface ControllerList {
  items: {
    metadata: { name: string; labels?: Record<string, string> };
    spec?: { replicas?: number };
    status?: { readyReplicas?: number };
  }[];
}

const WARNING = 'spinoza-e2e-warning';

const VIEWS = [
  { label: 'Cluster Overview', title: CONTEXT },
  { label: 'Issues', title: 'issues' },
  { label: 'Topology', title: 'topology' },
  { label: 'Helm releases', title: 'helm' },
  { label: 'Cluster checks', title: 'checks' },
];

function settingsWrite(page: Page) {
  return page.waitForResponse(
    (response) => response.url().includes('/api/settings') && response.request().method() === 'PUT',
    { timeout: 30_000 },
  );
}

test('the app opens on the cluster it was pointed at', async ({ page }) => {
  await openHome(page);
  await expect(page.getByRole('banner')).toContainText(CONTEXT);
  await expect(page).toHaveTitle(new RegExp(CONTEXT));
});

test('the app says the cluster feed is connected', async ({ page }) => {
  await openHome(page);
  await expect(page.getByRole('status', { name: 'The cluster feed is connected' })).toBeVisible();
});

test('the overview reports the Kubernetes version and node readiness from the cluster', async ({
  page,
}) => {
  const version = JSON.parse(kubectl(['version', '-o', 'json'])) as Version;
  const nodes = JSON.parse(kubectl(['get', 'nodes', '-o', 'json'])) as NodeList;
  const ready = nodes.items.filter((node) =>
    node.status?.conditions?.some(
      (condition) => condition.type === 'Ready' && condition.status === 'True',
    ),
  ).length;
  await openHome(page);
  const overview = page.getByRole('group', { name: 'Cluster overview' });
  const kubernetes = overview.getByText('Kubernetes', { exact: true }).locator('..');
  await expect(kubernetes.getByText(version.serverVersion.gitVersion, { exact: true })).toBeVisible(
    {
      timeout: 60_000,
    },
  );
  const nodeTile = overview.getByText('Nodes', { exact: true }).locator('..');
  await expect(nodeTile.getByText(String(nodes.items.length), { exact: true })).toBeVisible();
  await expect(nodeTile).toContainText(`${String(ready)} ready`);
});

test('the overview pod phases match the objects Kubernetes is serving', async ({ page }) => {
  const pods = JSON.parse(kubectl(['get', 'pods', '--all-namespaces', '-o', 'json'])) as PodList;
  const phases = { Running: 0, Pending: 0, Failed: 0, Succeeded: 0 };
  for (const pod of pods.items) {
    const phase = pod.status?.phase;
    if (phase === undefined) {
      continue;
    }
    if (!(phase in phases)) {
      continue;
    }
    phases[phase as keyof typeof phases] += 1;
  }
  await openHome(page);
  const overview = page.getByRole('group', { name: 'Cluster overview' });
  const podTile = overview.getByText('Pods', { exact: true }).locator('..');
  await expect(podTile.getByText(String(pods.items.length), { exact: true })).toBeVisible({
    timeout: 60_000,
  });
  await expect(podTile).toContainText(
    `${String(phases.Running)} running, ${String(phases.Pending)} pending, ${String(phases.Failed)} failed, ${String(phases.Succeeded)} succeeded`,
  );
});

test('the overview reports every installed GitOps controller at its live readiness', async ({
  page,
}) => {
  const controllers = JSON.parse(
    kubectl([
      'get',
      'deployments,statefulsets',
      '--all-namespaces',
      '-l',
      'app.kubernetes.io/part-of in (argocd,flux)',
      '-o',
      'json',
    ]),
  ) as ControllerList;
  expect(controllers.items.length).toBeGreaterThan(0);
  await openHome(page);
  const overview = page.getByRole('group', { name: 'Cluster overview' });
  await expect(overview.getByRole('heading', { name: 'GitOps controllers' })).toBeVisible({
    timeout: 60_000,
  });
  for (const controller of controllers.items) {
    const card = overview.getByText(controller.metadata.name, { exact: true }).locator('..');
    let ready = 0;
    if (controller.status?.readyReplicas !== undefined) {
      ready = controller.status.readyReplicas;
    }
    let wanted = 1;
    if (controller.spec?.replicas !== undefined) {
      wanted = controller.spec.replicas;
    }
    await expect(card).toContainText(`${String(ready)} of ${String(wanted)}`);
    const partOf = controller.metadata.labels?.['app.kubernetes.io/part-of'];
    if (partOf === 'argocd') {
      await expect(card).toContainText('Argo CD');
    }
    if (partOf === 'flux') {
      await expect(card).toContainText('Flux');
    }
  }
});

test('a live Kubernetes warning event is rendered with its evidence', async ({ page }) => {
  kubectl(['-n', NAMESPACE, 'delete', 'event', WARNING, '--ignore-not-found']);
  const uid = kubectl([
    '-n',
    NAMESPACE,
    'get',
    'configmap/config-sample',
    '-o',
    'jsonpath={.metadata.uid}',
  ]).trim();
  const now = new Date().toISOString();
  try {
    kubectlApply(
      JSON.stringify({
        apiVersion: 'v1',
        kind: 'Event',
        metadata: { name: WARNING, namespace: NAMESPACE },
        involvedObject: {
          apiVersion: 'v1',
          kind: 'ConfigMap',
          name: 'config-sample',
          namespace: NAMESPACE,
          uid,
        },
        reason: 'SpinozaE2EWarning',
        message: 'the overview reads this warning from Kubernetes',
        source: { component: 'spinoza-e2e' },
        firstTimestamp: now,
        lastTimestamp: now,
        count: 3,
        type: 'Warning',
      }),
    );
    expect(
      kubectl(['-n', NAMESPACE, 'get', `event/${WARNING}`, '-o', 'jsonpath={.reason}']).trim(),
    ).toBe('SpinozaE2EWarning');
    await openHome(page);
    const overview = page.getByRole('group', { name: 'Cluster overview' });
    const warning = overview.locator('tbody tr').filter({ hasText: 'SpinozaE2EWarning' });
    await expect(warning).toBeVisible({ timeout: 60_000 });
    await expect(warning).toContainText('ConfigMap/config-sample');
    await expect(warning).toContainText('the overview reads this warning from Kubernetes');
    await expect(warning).toContainText('3');
  } finally {
    kubectl(['-n', NAMESPACE, 'delete', 'event', WARNING, '--ignore-not-found']);
  }
});

for (const view of VIEWS) {
  test(`the sidebar reaches ${view.label} and the title follows`, async ({ page }) => {
    await openHome(page);
    await sidebar(page, view.label).click();
    await expect(page).toHaveTitle(new RegExp(`^${view.title} `));
  });
}

test('a view survives a reload because it lives in the URL', async ({ page }) => {
  await openView(page, 'checks');
  await page.reload();
  await expect(page).toHaveTitle(/^checks /);
});

test('the sidebar reflects the integrations available in the full profile', async ({ page }) => {
  await openHome(page);
  await expect(sidebar(page, 'Traffic')).toBeDisabled();
  for (const label of ['Flux', 'Argo CD']) {
    await expect(sidebar(page, label)).toBeEnabled();
  }
});

test('the desktop switch explains why it is unavailable in a browser', async ({ page }) => {
  await openHome(page);
  await expect(sidebar(page, 'Desktop')).toBeDisabled();
  await expect(page.getByRole('banner')).toContainText('only available when Spinoza starts');
});

test('every namespace in the cluster is offered for scoping', async ({ page }) => {
  await openHome(page);
  const picker = page.getByRole('combobox', { name: 'Namespace' });
  await expect(picker).toBeVisible();
  await expect(picker.getByRole('option', { name: 'e2e', exact: true })).toHaveCount(1);
  await expect(picker.getByRole('option', { name: 'All namespaces', exact: true })).toHaveCount(1);
});

test('the command palette opens on its shortcut', async ({ page }) => {
  await openHome(page);
  await primaryShortcut(page, 'k');
  await expect(page.getByPlaceholder('Search')).toBeVisible();
});

test('the page offers a skip link into its content', async ({ page }) => {
  await openHome(page);
  await expect(page.getByRole('link', { name: 'Skip to the content' })).toHaveAttribute(
    'href',
    '#content',
  );
});

test('the resource tree counts what it found and remembers its expanded category', async ({
  page,
}) => {
  await openHome(page);
  const workloads = page.getByRole('button', { name: /^Workloads \d+$/ });
  await expect(workloads).toBeVisible({ timeout: 60_000 });
  const original = await workloads.getAttribute('aria-expanded');
  if (original === null) {
    throw new Error('the Workloads category does not expose its expanded state');
  }
  try {
    if (original !== 'true') {
      const saved = settingsWrite(page);
      await workloads.click();
      await saved;
    }
    await expect(workloads).toHaveAttribute('aria-expanded', 'true');
    await expect(sidebar(page, /^Pod \d/)).toBeVisible();
    await expect(sidebar(page, /^Deployment \d/)).toBeVisible();
    await expect(sidebar(page, /^CronJob \d/)).toBeVisible();
    await page.reload();
    await expect(workloads).toHaveAttribute('aria-expanded', 'true', { timeout: 60_000 });
    await expect(sidebar(page, /^Pod \d/)).toBeVisible();
  } finally {
    if ((await workloads.getAttribute('aria-expanded')) !== original) {
      const restored = settingsWrite(page);
      await workloads.click();
      await restored;
      await expect(workloads).toHaveAttribute('aria-expanded', original);
    }
  }
});

test('browser back and forward restore complete application routes', async ({ page }) => {
  await openHome(page);
  await sidebar(page, 'Issues').click();
  await expect(page).toHaveTitle(/^issues /);
  await sidebar(page, 'Topology').click();
  await expect(page).toHaveTitle(/^topology /);
  await page.goBack();
  await expect(page).toHaveTitle(/^issues /);
  await page.goForward();
  await expect(page).toHaveTitle(/^topology /);
});

test('the help shortcut opens settings on the keyboard reference', async ({ page }) => {
  await openHome(page);
  await expect(page.getByRole('status', { name: 'The cluster feed is connected' })).toBeVisible();
  await page.keyboard.press('?');
  const dialog = page.getByRole('dialog', { name: 'Settings' });
  await expect(dialog).toBeVisible();
  await expect(
    dialog.getByRole('navigation', { name: 'Settings sections' }).getByRole('button', {
      name: 'Keyboard',
      exact: true,
    }),
  ).toHaveAttribute('aria-current', 'true');
  await expect(dialog.getByRole('table', { name: 'Keyboard shortcuts' })).toBeVisible();
});

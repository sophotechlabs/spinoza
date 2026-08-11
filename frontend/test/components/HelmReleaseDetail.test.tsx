import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { HelmRelease, HelmReleaseDetail as Detail } from '../../src/lib/types';
import HelmReleaseDetail from '../../src/components/HelmReleaseDetail';
import { useToastsStore } from '../../src/store/toasts';

const release: HelmRelease = {
  name: 'podinfo',
  namespace: 'demo',
  chart: 'podinfo',
  chartVersion: '6.9.2',
  appVersion: '6.9.2',
  revision: 3,
  status: 'deployed',
  updated: new Date(Date.now() - 3600000).toISOString(),
  description: 'Upgrade complete',
};

function detail(patch: Partial<Detail> = {}): Detail {
  return {
    release,
    driver: 'secret',
    firstDeployed: '2026-08-01T09:00:00Z',
    values: 'replicaCount: 2\n',
    notes: 'Thank you for installing podinfo.',
    manifest: 'apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: podinfo\n',
    resources: [
      {
        apiVersion: 'v1',
        kind: 'ConfigMap',
        name: 'podinfo',
        namespace: 'demo',
        version: 'v1',
        resource: 'configmaps',
      },
      { apiVersion: 'acme.io/v1', kind: 'Widget', name: 'thing' },
    ],
    history: [
      {
        revision: 3,
        status: 'deployed',
        chartVersion: '6.9.2',
        appVersion: '6.9.2',
        updated: release.updated,
        description: 'Upgrade complete',
      },
      {
        revision: 2,
        status: 'superseded',
        chartVersion: '6.9.1',
        appVersion: '6.9.1',
        updated: release.updated,
      },
    ],
    ...patch,
  };
}

interface Stubs {
  detail?: Detail;
  detailStatus?: number;
  support?: { available: boolean; reason?: string; binary: string };
  actionStatus?: number;
  actionBody?: unknown;
}

function stub(options: Stubs = {}) {
  const calls: { url: string; method: string }[] = [];
  const fetchMock = vi.fn((url: string, init?: { method?: string }) => {
    calls.push({ url, method: init?.method ?? 'GET' });
    if (url.startsWith('/api/helm/support')) {
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve(options.support ?? { available: true, binary: 'helm' }),
      });
    }
    if (url.startsWith('/api/helm/action')) {
      const status = options.actionStatus ?? 200;
      return Promise.resolve({
        ok: status === 200,
        status,
        json: () =>
          Promise.resolve(
            options.actionBody ?? { action: 'rollback', message: 'rolled back', revision: 2 },
          ),
      });
    }
    const status = options.detailStatus ?? 200;
    return Promise.resolve({
      ok: status === 200,
      status,
      json: () => Promise.resolve(options.detail ?? detail()),
    });
  });
  vi.stubGlobal('fetch', fetchMock);
  return calls;
}

function renderDetail() {
  const onSelectResource = vi.fn();
  const onChanged = vi.fn();
  const onClose = vi.fn();
  render(
    <HelmReleaseDetail
      release={release}
      onSelectResource={onSelectResource}
      onChanged={onChanged}
      onClose={onClose}
    />,
  );
  return { onSelectResource, onChanged, onClose };
}

beforeEach(() => {
  useToastsStore.getState().clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('HelmReleaseDetail', () => {
  it('opens on the overview with the release facts', async () => {
    stub();
    renderDetail();

    expect(await screen.findByText('Upgrade complete')).toBeInTheDocument();
    expect(screen.getByText('2026-08-01T09:00:00Z')).toBeInTheDocument();
    expect(screen.getByText('secrets')).toBeInTheDocument();
  });

  it('shows the supplied values', async () => {
    const user = userEvent.setup();
    stub();
    renderDetail();
    await screen.findByText('Upgrade complete');

    await user.click(screen.getByRole('button', { name: 'Values' }));

    expect(screen.getByText(/replicaCount: 2/)).toBeInTheDocument();
  });

  it('says when a release only used the chart defaults', async () => {
    const user = userEvent.setup();
    stub({ detail: detail({ values: '' }) });
    renderDetail();
    await screen.findByText('Upgrade complete');

    await user.click(screen.getByRole('button', { name: 'Values' }));

    expect(
      screen.getByText('This release was installed with the chart defaults.'),
    ).toBeInTheDocument();
  });

  it('shows the notes and the manifest', async () => {
    const user = userEvent.setup();
    stub();
    renderDetail();
    await screen.findByText('Upgrade complete');

    await user.click(screen.getByRole('button', { name: 'Notes' }));
    expect(screen.getByText(/Thank you for installing podinfo/)).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Manifest' }));
    expect(screen.getByText(/kind: ConfigMap/)).toBeInTheDocument();
  });

  it('opens a rendered resource in the inspector', async () => {
    const user = userEvent.setup();
    stub();
    const { onSelectResource } = renderDetail();
    await screen.findByText('Upgrade complete');

    await user.click(screen.getByRole('button', { name: 'Resources' }));
    await user.click(screen.getByRole('button', { name: 'podinfo' }));

    expect(onSelectResource).toHaveBeenCalledWith({
      group: '',
      version: 'v1',
      resource: 'configmaps',
      namespace: 'demo',
      name: 'podinfo',
    });
  });

  it('leaves a kind the cluster does not report unopenable', async () => {
    const user = userEvent.setup();
    stub();
    renderDetail();
    await screen.findByText('Upgrade complete');

    await user.click(screen.getByRole('button', { name: 'Resources' }));

    expect(screen.queryByRole('button', { name: 'thing' })).not.toBeInTheDocument();
    expect(screen.getByText('thing')).toBeInTheDocument();
  });

  it('lists the revision history newest first and offers a rollback', async () => {
    const user = userEvent.setup();
    const calls = stub();
    const { onChanged } = renderDetail();
    await screen.findByText('Upgrade complete');

    await user.click(screen.getByRole('button', { name: 'History' }));
    expect(screen.getByText('superseded')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Roll back' }));

    await waitFor(() => {
      expect(onChanged).toHaveBeenCalled();
    });
    const action = calls.find((call) => call.url.startsWith('/api/helm/action'));
    expect(action?.method).toBe('POST');
    expect(action?.url).toContain('action=rollback');
    expect(action?.url).toContain('revision=2');
    expect(useToastsStore.getState().toasts.at(-1)?.message).toBe('rolled back');
  });

  it('offers no rollback for the revision already deployed', async () => {
    const user = userEvent.setup();
    stub();
    renderDetail();
    await screen.findByText('Upgrade complete');

    await user.click(screen.getByRole('button', { name: 'History' }));

    expect(screen.getAllByRole('button', { name: 'Roll back' })).toHaveLength(1);
  });

  it('asks before uninstalling and only then calls helm', async () => {
    const user = userEvent.setup();
    const calls = stub({ actionBody: { action: 'uninstall', message: 'release removed' } });
    const { onClose, onChanged } = renderDetail();
    await screen.findByText('Upgrade complete');

    await user.click(screen.getByRole('button', { name: 'Uninstall' }));
    expect(screen.getByText('Uninstall podinfo? This cannot be undone.')).toBeInTheDocument();
    expect(calls.some((call) => call.url.startsWith('/api/helm/action'))).toBe(false);

    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    await waitFor(() => {
      expect(onClose).toHaveBeenCalled();
    });
    expect(onChanged).toHaveBeenCalled();
    const action = calls.find((call) => call.url.startsWith('/api/helm/action'));
    expect(action?.url).toContain('action=uninstall');
  });

  it('backs out of an uninstall on cancel', async () => {
    const user = userEvent.setup();
    const calls = stub();
    renderDetail();
    await screen.findByText('Upgrade complete');

    await user.click(screen.getByRole('button', { name: 'Uninstall' }));
    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(screen.getByRole('button', { name: 'Uninstall' })).toBeInTheDocument();
    expect(calls.some((call) => call.url.startsWith('/api/helm/action'))).toBe(false);
  });

  it('disables both actions when helm is missing and says why', async () => {
    stub({ support: { available: false, reason: 'helm was not found on PATH', binary: 'helm' } });
    renderDetail();

    expect(await screen.findByText(/helm was not found on PATH/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Uninstall' })).toBeDisabled();
  });

  it('surfaces a failed action without closing', async () => {
    const user = userEvent.setup();
    stub({ actionStatus: 500, actionBody: { message: 'release: not found' } });
    const { onClose } = renderDetail();
    await screen.findByText('Upgrade complete');

    await user.click(screen.getByRole('button', { name: 'Uninstall' }));
    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    expect(await screen.findByText('release: not found')).toBeInTheDocument();
    expect(onClose).not.toHaveBeenCalled();
  });

  it('shows why the detail could not be loaded', async () => {
    stub({ detailStatus: 404, detail: detail() });
    renderDetail();

    expect(await screen.findByRole('alert')).toBeInTheDocument();
  });

  it('carries a partial-data note from the server', async () => {
    stub({ detail: detail({ error: 'the newest revision could not be read' }) });
    renderDetail();

    expect(await screen.findByText('the newest revision could not be read')).toBeInTheDocument();
  });

  it('closes on the close button', async () => {
    const user = userEvent.setup();
    stub();
    const { onClose } = renderDetail();
    await screen.findByText('Upgrade complete');

    await user.click(screen.getByRole('button', { name: 'Close the release detail' }));

    expect(onClose).toHaveBeenCalled();
  });

  it('says when a release rendered nothing to show', async () => {
    const user = userEvent.setup();
    stub({ detail: detail({ resources: [], notes: '', manifest: '', history: [] }) });
    renderDetail();
    await screen.findByText('Upgrade complete');

    await user.click(screen.getByRole('button', { name: 'Resources' }));
    expect(screen.getByText('This release rendered no resources.')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Notes' }));
    expect(screen.getByText('This chart renders no notes.')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Manifest' }));
    expect(screen.getByText('This release rendered no manifest.')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'History' }));
    expect(screen.getByText('This release has no stored revisions.')).toBeInTheDocument();
  });
});

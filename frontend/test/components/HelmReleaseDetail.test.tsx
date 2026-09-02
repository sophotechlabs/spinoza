import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { HelmRelease, HelmReleaseDetail as Detail } from '../../src/lib/types';
import HelmReleaseDetail from '../../src/components/HelmReleaseDetail';
import { useToastsStore } from '../../src/store/toasts';
import { capabilities } from '../helpers';
import { useContextsStore } from '../../src/store/contexts';
import { useHelmStore } from '../../src/store/helm';
import { useHelmAccessStore } from '../../src/store/helmAccess';
import { bumpClusterEpoch } from '../../src/store/cluster';

vi.mock('../../src/components/HelmUpgradeDialog', () => ({
  default: ({
    release: releaseProp,
    currentValues,
    onClose,
    onUpgraded,
  }: {
    release: HelmRelease;
    currentValues: string;
    onClose: () => void;
    onUpgraded: () => void;
  }) => (
    <div data-testid="upgrade-dialog" data-release={releaseProp.name} data-values={currentValues}>
      <button type="button" onClick={onUpgraded}>
        finish-upgrade
      </button>
      <button type="button" onClick={onClose}>
        close-upgrade
      </button>
    </div>
  ),
}));

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

interface Refusal {
  capability: string;
  reason: string;
}

interface Stubs {
  refused?: Refusal[];
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
    if (url.startsWith('/api/capabilities')) {
      return Promise.resolve({
        ok: true,
        json: () =>
          Promise.resolve(
            capabilities({
              helm: options.support ?? { available: true, reason: undefined, binary: 'helm' },
            }),
          ),
      });
    }
    if (url.startsWith('/api/helm/access')) {
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ refused: options.refused ?? [] }),
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
  const onOpenResource = vi.fn();
  const onClose = vi.fn();
  render(
    <HelmReleaseDetail
      namespace="demo"
      name="podinfo"
      onSelectResource={onSelectResource}
      onOpenResource={onOpenResource}
      onClose={onClose}
    />,
  );
  return { onSelectResource, onOpenResource, onClose };
}

beforeEach(() => {
  useToastsStore.getState().clear();
  useHelmAccessStore.setState({ answers: {} });
});

afterEach(() => {
  vi.unstubAllGlobals();
  act(() => {
    useToastsStore.getState().clear();
  });
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

  it('opens a rendered resource in its table', async () => {
    const user = userEvent.setup();
    stub();
    const { onOpenResource } = renderDetail();
    await screen.findByText('Upgrade complete');

    await user.click(screen.getByRole('button', { name: 'Resources' }));
    await user.click(screen.getByRole('button', { name: 'podinfo' }));

    expect(onOpenResource).toHaveBeenCalledWith(
      {
        group: '',
        version: 'v1',
        resource: 'configmaps',
        namespace: 'demo',
        name: 'podinfo',
      },
      'ConfigMap',
    );
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
    renderDetail();
    await screen.findByText('Upgrade complete');

    await user.click(screen.getByRole('button', { name: 'History' }));
    expect(screen.getByText('superseded')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Roll back' }));

    await waitFor(() => {
      expect(useHelmStore.getState().epoch).toBe(1);
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
    const { onClose } = renderDetail();
    await screen.findByText('Upgrade complete');

    await user.click(screen.getByRole('button', { name: 'Uninstall' }));
    expect(screen.getByText('Uninstall podinfo? This cannot be undone.')).toBeInTheDocument();
    expect(calls.some((call) => call.url.startsWith('/api/helm/action'))).toBe(false);

    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    await waitFor(() => {
      expect(onClose).toHaveBeenCalled();
    });
    expect(useHelmStore.getState().epoch).toBe(1);
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

  it('disables the actions when helm is missing and says why', async () => {
    stub({ support: { available: false, reason: 'helm was not found on PATH', binary: 'helm' } });
    renderDetail();

    expect(await screen.findByText(/helm was not found on PATH/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Uninstall' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Upgrade' })).toBeDisabled();
  });

  it('opens the upgrade dialog seeded with the loaded release', async () => {
    const user = userEvent.setup();
    const calls = stub();
    renderDetail();
    await screen.findByText('Upgrade complete');

    await user.click(screen.getByRole('button', { name: 'Upgrade' }));

    const dialog = screen.getByTestId('upgrade-dialog');
    expect(dialog.dataset.release).toBe('podinfo');
    expect(dialog.dataset.values).toBe('replicaCount: 2\n');

    const before = calls.filter((call) => call.url.startsWith('/api/helm/release')).length;
    await user.click(screen.getByRole('button', { name: 'finish-upgrade' }));

    expect(useHelmStore.getState().epoch).toBe(1);
    await waitFor(() => {
      const after = calls.filter((call) => call.url.startsWith('/api/helm/release')).length;
      expect(after).toBe(before + 1);
    });

    await user.click(screen.getByRole('button', { name: 'close-upgrade' }));
    expect(screen.queryByTestId('upgrade-dialog')).not.toBeInTheDocument();
  });

  it('closes an upgrade dialog when the cluster reconnects', async () => {
    const user = userEvent.setup();
    stub();
    renderDetail();
    await screen.findByText('Upgrade complete');
    await user.click(screen.getByRole('button', { name: 'Upgrade' }));
    expect(screen.getByTestId('upgrade-dialog')).toBeInTheDocument();

    await act(async () => {
      bumpClusterEpoch();
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(screen.queryByTestId('upgrade-dialog')).not.toBeInTheDocument();
  });

  it('drops a release action that finishes after the cluster reconnects', async () => {
    const user = userEvent.setup();
    let finishAction!: (response: { ok: boolean; json: () => Promise<unknown> }) => void;
    const action = new Promise<{ ok: boolean; json: () => Promise<unknown> }>((resolve) => {
      finishAction = resolve;
    });
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.startsWith('/api/capabilities')) {
          return Promise.resolve({ ok: true, json: () => Promise.resolve(capabilities()) });
        }
        if (url.startsWith('/api/helm/access')) {
          return Promise.resolve({ ok: true, json: () => Promise.resolve({ refused: [] }) });
        }
        if (url.startsWith('/api/helm/action')) {
          return action;
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve(detail()) });
      }),
    );
    const { onClose } = renderDetail();
    await screen.findByText('Upgrade complete');
    await user.click(screen.getByRole('button', { name: 'Uninstall' }));
    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    await act(async () => {
      bumpClusterEpoch();
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(screen.queryByText(/Uninstall podinfo/)).not.toBeInTheDocument();

    await act(async () => {
      finishAction({
        ok: true,
        json: () => Promise.resolve({ action: 'uninstall', message: 'old cluster removed' }),
      });
      await action;
      await Promise.resolve();
    });

    expect(onClose).not.toHaveBeenCalled();
    expect(useHelmStore.getState().epoch).toBe(0);
    expect(useToastsStore.getState().toasts).toHaveLength(0);
  });

  it('drops a release action failure after the cluster reconnects', async () => {
    const user = userEvent.setup();
    let failAction!: (reason: unknown) => void;
    const action = new Promise((_resolve, reject) => {
      failAction = reject;
    });
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.startsWith('/api/capabilities')) {
          return Promise.resolve({ ok: true, json: () => Promise.resolve(capabilities()) });
        }
        if (url.startsWith('/api/helm/access')) {
          return Promise.resolve({ ok: true, json: () => Promise.resolve({ refused: [] }) });
        }
        if (url.startsWith('/api/helm/action')) {
          return action;
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve(detail()) });
      }),
    );
    const { onClose } = renderDetail();
    await screen.findByText('Upgrade complete');
    await user.click(screen.getByRole('button', { name: 'Uninstall' }));
    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    await act(async () => {
      bumpClusterEpoch();
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    await act(async () => {
      failAction(new Error('old cluster failed'));
      await Promise.resolve();
    });

    expect(onClose).not.toHaveBeenCalled();
    expect(useHelmStore.getState().epoch).toBe(0);
    expect(useToastsStore.getState().toasts).toHaveLength(0);
  });

  it('swaps the upgrade button for the flux owner', async () => {
    const user = userEvent.setup();
    const fluxRef = {
      group: 'helm.toolkit.fluxcd.io',
      version: 'v2',
      resource: 'helmreleases',
      namespace: 'demo',
      name: 'podinfo',
    };
    stub({ detail: detail({ release: { ...release, fluxRef } }) });
    const { onSelectResource } = renderDetail();
    await screen.findByText('Upgrade complete');

    expect(screen.queryByRole('button', { name: 'Upgrade' })).not.toBeInTheDocument();
    expect(screen.getByText('Flux demo/podinfo')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Managed by Flux' }));

    expect(onSelectResource).toHaveBeenCalledWith(fluxRef);
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

describe('HelmReleaseDetail on a protected cluster', () => {
  const showModal = vi.fn(function showModal(this: HTMLDialogElement) {
    this.open = true;
  });
  const close = vi.fn(function close(this: HTMLDialogElement) {
    this.open = false;
  });

  beforeEach(() => {
    useToastsStore.getState().clear();
    showModal.mockClear();
    close.mockClear();
    HTMLDialogElement.prototype.showModal = showModal;
    HTMLDialogElement.prototype.close = close;
    useContextsStore.getState().setList({
      current: { kubeconfig: '', name: 'p-mk1' },
      kubeconfigs: [],
      protection: 'protected',
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    act(() => {
      useToastsStore.getState().clear();
    });
  });

  it('asks for the release name before uninstalling', async () => {
    const user = userEvent.setup();
    const calls = stub({ actionBody: { action: 'uninstall', message: 'gone' } });
    const { onClose } = renderDetail();
    await screen.findByText('Upgrade complete');

    await user.click(screen.getByRole('button', { name: 'Uninstall' }));
    expect(screen.getByText('Uninstalling podinfo. This cannot be undone.')).toBeInTheDocument();
    expect(calls.some((call) => call.url.startsWith('/api/helm/action'))).toBe(false);

    await user.type(screen.getByLabelText('Name'), 'podinfo');
    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    await waitFor(() => {
      expect(onClose).toHaveBeenCalled();
    });
    const action = calls.find((call) => call.url.startsWith('/api/helm/action'));
    expect(action?.url).toContain('confirm=podinfo');
  });

  it('asks for the release name before rolling back', async () => {
    const user = userEvent.setup();
    const calls = stub();
    renderDetail();
    await screen.findByText('Upgrade complete');

    await user.click(screen.getByRole('button', { name: 'History' }));
    await user.click(screen.getByRole('button', { name: 'Roll back' }));
    expect(screen.getByText('Rolling podinfo back to revision 2.')).toBeInTheDocument();

    await user.type(screen.getByLabelText('Name'), 'podinfo');
    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    await waitFor(() => {
      expect(useHelmStore.getState().epoch).toBe(1);
    });
    const action = calls.find((call) => call.url.startsWith('/api/helm/action'));
    expect(action?.url).toContain('revision=2');
    expect(action?.url).toContain('confirm=podinfo');
  });

  it('drops the question when it is cancelled', async () => {
    const user = userEvent.setup();
    const calls = stub();
    renderDetail();
    await screen.findByText('Upgrade complete');

    await user.click(screen.getByRole('button', { name: 'Uninstall' }));
    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(screen.queryByLabelText('Name')).not.toBeInTheDocument();
    expect(calls.some((call) => call.url.startsWith('/api/helm/action'))).toBe(false);
  });
});

describe('HelmReleaseDetail when the cluster refuses', () => {
  it('holds back Upgrade and says why', async () => {
    stub({ refused: [{ capability: 'upgrade', reason: 'no creating secrets in demo' }] });
    renderDetail();

    const upgrade = await screen.findByRole('button', { name: 'Upgrade' });

    await waitFor(() => {
      expect(upgrade).toBeDisabled();
    });
    expect(upgrade).toHaveAttribute('title', 'no creating secrets in demo');
  });

  it('holds back Uninstall and says why', async () => {
    stub({ refused: [{ capability: 'uninstall', reason: 'no deleting secrets in demo' }] });
    renderDetail();

    const uninstall = await screen.findByRole('button', { name: 'Uninstall' });

    await waitFor(() => {
      expect(uninstall).toBeDisabled();
    });
    expect(uninstall).toHaveAttribute('title', 'no deleting secrets in demo');
  });

  it('holds back every Roll back and says why', async () => {
    const user = userEvent.setup();
    stub({ refused: [{ capability: 'rollback', reason: 'no creating secrets in demo' }] });
    renderDetail();
    await user.click(await screen.findByRole('button', { name: 'History' }));

    const rollback = await screen.findByRole('button', { name: 'Roll back' });

    await waitFor(() => {
      expect(rollback).toBeDisabled();
    });
    expect(rollback).toHaveAttribute('title', 'no creating secrets in demo');
  });

  it('leaves the actions it was not told about alone', async () => {
    stub({ refused: [{ capability: 'upgrade', reason: 'no creating secrets in demo' }] });
    renderDetail();

    const uninstall = await screen.findByRole('button', { name: 'Uninstall' });

    await waitFor(() => {
      expect(uninstall).toBeEnabled();
    });
  });

  it('leaves every button alone when nothing is refused', async () => {
    stub();
    renderDetail();

    const upgrade = await screen.findByRole('button', { name: 'Upgrade' });

    await waitFor(() => {
      expect(upgrade).toBeEnabled();
    });
    expect(upgrade).toHaveAttribute('title', 'Upgrade this release');
  });

  it('asks about the release that is open', async () => {
    const calls = stub();
    renderDetail();

    await waitFor(() => {
      expect(
        calls.some((call) => call.url === '/api/helm/access?namespace=demo&name=podinfo'),
      ).toBe(true);
    });
  });

  it('says helm is missing before it says the cluster refused', async () => {
    stub({
      support: { available: false, reason: 'helm was not found on PATH', binary: 'helm' },
      refused: [{ capability: 'uninstall', reason: 'no deleting secrets in demo' }],
    });
    renderDetail();

    const uninstall = await screen.findByRole('button', { name: 'Uninstall' });

    await waitFor(() => {
      expect(uninstall).toBeDisabled();
    });
    expect(uninstall).toHaveAttribute('title', 'helm was not found on PATH');
  });
});

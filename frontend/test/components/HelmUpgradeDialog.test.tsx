import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { HelmRelease } from '../../src/lib/types';
import HelmUpgradeDialog from '../../src/components/HelmUpgradeDialog';
import { useToastsStore } from '../../src/store/toasts';
import { useContextsStore } from '../../src/store/contexts';

vi.mock('../../src/lib/monaco', () => ({
  defineEditorTheme: vi.fn(),
}));

vi.mock('@monaco-editor/react', () => ({
  default: ({
    value,
    onChange,
    options,
  }: {
    value: string;
    onChange: (next: string | undefined) => void;
    options: { readOnly: boolean };
  }) => (
    <textarea
      aria-label="yaml"
      readOnly={options.readOnly}
      value={value}
      onChange={(event) => {
        onChange(event.target.value);
      }}
    />
  ),
  DiffEditor: ({ original, modified }: { original: string; modified: string }) => (
    <div data-testid="manifest-diff" data-original={original} data-modified={modified} />
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
  updated: '2026-08-11T09:30:00Z',
};

const versionsPayload = {
  chart: 'podinfo',
  repos: [
    { name: 'podinfo', url: 'https://example.com', versions: ['6.15.1', '6.14.0'] },
    { url: 'oci://ghcr.io/acme/charts', oci: true, versions: ['1.0.0'] },
  ],
};

interface Stubs {
  versions?: unknown;
  versionsStatus?: number;
  upgradeStatus?: number;
  upgradeBody?: unknown;
}

function stub(options: Stubs = {}) {
  const calls: { url: string; method: string; body: string }[] = [];
  const fetchMock = vi.fn((url: string, init?: { method?: string; body?: string }) => {
    calls.push({ url, method: init?.method ?? 'GET', body: init?.body ?? '' });
    if (url.startsWith('/api/helm/versions')) {
      const status = options.versionsStatus ?? 200;
      return Promise.resolve({
        ok: status === 200,
        status,
        json: () => Promise.resolve(options.versions ?? versionsPayload),
      });
    }
    const status = options.upgradeStatus ?? 200;
    return Promise.resolve({
      ok: status === 200,
      status,
      json: () =>
        Promise.resolve(options.upgradeBody ?? { action: 'upgrade', message: 'upgraded' }),
    });
  });
  vi.stubGlobal('fetch', fetchMock);
  return calls;
}

function renderDialog() {
  const onClose = vi.fn();
  const onUpgraded = vi.fn();
  render(
    <HelmUpgradeDialog
      release={release}
      currentValues={'replicaCount: 2\n'}
      currentManifest={'kind: ConfigMap\nmetadata:\n  name: old\n'}
      onClose={onClose}
      onUpgraded={onUpgraded}
    />,
  );
  return { onClose, onUpgraded };
}

async function pickAndPreview(user: ReturnType<typeof userEvent.setup>) {
  await screen.findByLabelText('Chart version');
  await user.selectOptions(screen.getByLabelText('Chart version'), '0:6.15.1');
  await user.click(screen.getByRole('button', { name: 'Preview' }));
}

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
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('HelmUpgradeDialog', () => {
  it('loads the versions and groups them by repository', async () => {
    stub();
    renderDialog();

    expect(screen.getByText('Loading versions')).toBeInTheDocument();

    const select = await screen.findByLabelText('Chart version');
    const groups = select.querySelectorAll('optgroup');
    expect(groups).toHaveLength(2);
    expect(groups[0].label).toBe('podinfo');
    expect(groups[1].label).toBe('oci://ghcr.io/acme/charts');
    const versions = [...select.querySelectorAll('option')].map((option) => option.textContent);
    expect(versions).toEqual(['pick a version', '6.15.1', '6.14.0', '1.0.0']);
    expect(screen.getByText('from 6.9.2')).toBeInTheDocument();
  });

  it('says when the version lookup fails', async () => {
    stub({ versionsStatus: 500, versions: { message: 'no cluster' } });
    renderDialog();

    expect(await screen.findByRole('alert')).toHaveTextContent('no cluster');
  });

  it('says when no repository offers the chart', async () => {
    stub({
      versions: { chart: 'podinfo', repos: [], error: 'no chart repositories are configured' },
    });
    renderDialog();

    expect(await screen.findByText('no chart repositories are configured')).toBeInTheDocument();
  });

  it('falls back to its own words when the chart is simply unknown', async () => {
    stub({ versions: { chart: 'podinfo', repos: [] } });
    renderDialog();

    expect(
      await screen.findByText('no configured chart repository offers this chart'),
    ).toBeInTheDocument();
  });

  it('carries a partial failure note next to the picker', async () => {
    stub({ versions: { ...versionsPayload, error: 'one repository failed' } });
    renderDialog();

    expect(await screen.findByText('one repository failed')).toBeInTheDocument();
    expect(screen.getByLabelText('Chart version')).toBeInTheDocument();
  });

  it('keeps the preview shut until a version is picked', async () => {
    stub();
    renderDialog();

    const preview = await screen.findByRole('button', { name: 'Preview' });
    expect(preview).toBeDisabled();
  });

  it('previews a server render of the edited values', async () => {
    const user = userEvent.setup();
    const calls = stub({
      upgradeBody: {
        action: 'upgrade',
        dryRun: true,
        manifest: 'kind: ConfigMap\nmetadata:\n  name: new\n',
      },
    });
    renderDialog();
    await screen.findByLabelText('Chart version');

    await user.clear(await screen.findByLabelText('yaml'));
    await user.type(screen.getByLabelText('yaml'), 'replicaCount: 3');
    await pickAndPreview(user);

    const diff = await screen.findByTestId('manifest-diff');
    expect(diff.dataset.original).toContain('name: old');
    expect(diff.dataset.modified).toContain('name: new');
    const call = calls.find((one) => one.url.startsWith('/api/helm/upgrade'));
    expect(call?.url).toBe('/api/helm/upgrade?dryRun=true');
    expect(JSON.parse(call?.body ?? '{}')).toEqual({
      namespace: 'demo',
      name: 'podinfo',
      chart: 'podinfo',
      repo: 'https://example.com',
      version: '6.15.1',
      values: 'replicaCount: 3',
    });
  });

  it('stays on the editor when the preview fails', async () => {
    const user = userEvent.setup();
    stub({ upgradeStatus: 400, upgradeBody: { message: 'version "x" is not a semantic version' } });
    renderDialog();

    await pickAndPreview(user);

    expect(await screen.findByText('version "x" is not a semantic version')).toBeInTheDocument();
    expect(screen.queryByTestId('manifest-diff')).not.toBeInTheDocument();
    expect(screen.getByLabelText('yaml')).toBeInTheDocument();
  });

  it('walks back from the diff to the editor', async () => {
    const user = userEvent.setup();
    stub({ upgradeBody: { action: 'upgrade', dryRun: true, manifest: 'new' } });
    renderDialog();

    await pickAndPreview(user);
    await screen.findByTestId('manifest-diff');
    await user.click(screen.getByRole('button', { name: 'Back' }));

    expect(screen.getByLabelText('yaml')).toBeInTheDocument();
    expect(screen.queryByTestId('manifest-diff')).not.toBeInTheDocument();
  });

  it('upgrades after the diff and reports what helm said', async () => {
    const user = userEvent.setup();
    const calls = stub({ upgradeBody: { action: 'upgrade', message: 'upgraded', dryRun: false } });
    const { onClose, onUpgraded } = renderDialog();

    await pickAndPreview(user);
    await screen.findByRole('button', { name: 'Upgrade to 6.15.1' });
    await user.click(screen.getByRole('button', { name: 'Upgrade to 6.15.1' }));

    await waitFor(() => {
      expect(onUpgraded).toHaveBeenCalled();
    });
    expect(onClose).toHaveBeenCalled();
    const real = calls.filter((one) => one.url.startsWith('/api/helm/upgrade')).at(-1);
    expect(real?.url).toBe('/api/helm/upgrade');
    expect(useToastsStore.getState().toasts.at(-1)?.message).toBe('upgraded');
  });

  it('keeps the dialog open when the upgrade fails', async () => {
    const user = userEvent.setup();
    let status = 200;
    const calls: { url: string }[] = [];
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: { method?: string }) => {
        calls.push({ url });
        if (url.startsWith('/api/helm/versions')) {
          return Promise.resolve({ ok: true, json: () => Promise.resolve(versionsPayload) });
        }
        if (init?.method === 'POST' && !url.includes('dryRun')) {
          status = 409;
          return Promise.resolve({
            ok: false,
            status,
            json: () => Promise.resolve({ message: 'managed by flux' }),
          });
        }
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ action: 'upgrade', dryRun: true, manifest: 'new' }),
        });
      }),
    );
    const { onClose } = renderDialog();

    await pickAndPreview(user);
    await screen.findByRole('button', { name: 'Upgrade to 6.15.1' });
    await user.click(screen.getByRole('button', { name: 'Upgrade to 6.15.1' }));

    expect(await screen.findByText('managed by flux')).toBeInTheDocument();
    expect(onClose).not.toHaveBeenCalled();
  });

  it('closes on cancel without touching the cluster', async () => {
    const user = userEvent.setup();
    const calls = stub();
    const { onClose } = renderDialog();
    await screen.findByLabelText('Chart version');

    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(onClose).toHaveBeenCalled();
    expect(calls.some((one) => one.url.startsWith('/api/helm/upgrade'))).toBe(false);
  });

  it('sends the oci repository through untouched', async () => {
    const user = userEvent.setup();
    const calls = stub({ upgradeBody: { action: 'upgrade', dryRun: true, manifest: 'new' } });
    renderDialog();
    await screen.findByLabelText('Chart version');

    await user.selectOptions(screen.getByLabelText('Chart version'), '1:1.0.0');
    await user.click(screen.getByRole('button', { name: 'Preview' }));

    const call = calls.find((one) => one.url.startsWith('/api/helm/upgrade'));
    const body = JSON.parse(call?.body ?? '{}') as { repo: string; version: string };
    expect(body.repo).toBe('oci://ghcr.io/acme/charts');
    expect(body.version).toBe('1.0.0');
  });
});

describe('HelmUpgradeDialog on a protected cluster', () => {
  beforeEach(() => {
    useContextsStore.getState().setList({
      current: { kubeconfig: '', name: 'p-mk1' },
      kubeconfigs: [],
      protection: 'protected',
    });
  });

  it('asks for the release name before upgrading', async () => {
    const user = userEvent.setup();
    const calls = stub({ upgradeBody: { action: 'upgrade', dryRun: true, manifest: 'new' } });
    const { onClose } = renderDialog();

    await pickAndPreview(user);
    await screen.findByRole('button', { name: 'Upgrade to 6.15.1' });
    await user.click(screen.getByRole('button', { name: 'Upgrade to 6.15.1' }));

    expect(screen.getByText(/Upgrading podinfo to podinfo 6\.15\.1/)).toBeInTheDocument();
    expect(calls.filter((one) => one.url === '/api/helm/upgrade')).toHaveLength(0);

    await user.type(screen.getByLabelText('Name'), 'podinfo');
    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    await waitFor(() => {
      expect(onClose).toHaveBeenCalled();
    });
    const real = calls.at(-1);
    expect(real?.url).toBe('/api/helm/upgrade?confirm=podinfo');
  });

  it('drops the question when it is cancelled', async () => {
    const user = userEvent.setup();
    const calls = stub({ upgradeBody: { action: 'upgrade', dryRun: true, manifest: 'new' } });
    renderDialog();

    await pickAndPreview(user);
    await screen.findByRole('button', { name: 'Upgrade to 6.15.1' });
    await user.click(screen.getByRole('button', { name: 'Upgrade to 6.15.1' }));
    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(screen.queryByLabelText('Name')).not.toBeInTheDocument();
    expect(calls.filter((one) => one.url === '/api/helm/upgrade')).toHaveLength(0);
  });
});

describe('a dry run on a protected cluster', () => {
  beforeEach(() => {
    useContextsStore.getState().setList({
      current: { kubeconfig: '', name: 'p-mk1' },
      kubeconfigs: [],
      protection: 'protected',
    });
  });

  it('needs no typed name for the preview itself', async () => {
    const user = userEvent.setup();
    const calls = stub({ upgradeBody: { action: 'upgrade', dryRun: true, manifest: 'new' } });
    renderDialog();

    await pickAndPreview(user);

    await screen.findByTestId('manifest-diff');
    const preview = calls.find((one) => one.url.startsWith('/api/helm/upgrade'));
    expect(preview?.url).toBe('/api/helm/upgrade?dryRun=true');
  });
});

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ArgoActions from '../../src/components/ArgoActions';
import type { ObjectRef } from '../../src/lib/types';
import { EMPTY_CONTEXTS, useContextsStore } from '../../src/store/contexts';
import { accessKey, useAccessStore } from '../../src/store/access';

const target: ObjectRef = {
  group: 'argoproj.io',
  version: 'v1alpha1',
  resource: 'applications',
  namespace: 'argocd',
  name: 'podinfo',
};

const showModal = vi.fn(function showModal(this: HTMLDialogElement) {
  this.open = true;
});

const close = vi.fn(function close(this: HTMLDialogElement) {
  this.open = false;
});

function renderActions() {
  const onDone = vi.fn();
  const view = render(<ArgoActions target={target} onDone={onDone} />);
  return { onDone, view };
}

function lastCallUrl(): string {
  const mock = globalThis.fetch as ReturnType<typeof vi.fn>;
  return mock.mock.calls[mock.mock.calls.length - 1][0] as string;
}

function openCluster() {
  useContextsStore.getState().setList({
    current: { kubeconfig: '', name: 'p-mk1' },
    kubeconfigs: [],
    protection: 'open',
  });
}

function protectedCluster() {
  useContextsStore.getState().setList({
    current: { kubeconfig: '', name: 'p-mk1' },
    kubeconfigs: [],
    protection: 'protected',
  });
}

describe('the argo cd actions on an application', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({ action: 'sync' }) }),
    );
    showModal.mockClear();
    close.mockClear();
    HTMLDialogElement.prototype.showModal = showModal;
    HTMLDialogElement.prototype.close = close;
    openCluster();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('offers both a sync and a refresh', () => {
    renderActions();

    expect(screen.getByRole('button', { name: 'Sync' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Refresh' })).toBeInTheDocument();
  });

  it('syncs straight away on an open cluster', async () => {
    const user = userEvent.setup();
    const { onDone } = renderActions();

    await user.click(screen.getByRole('button', { name: 'Sync' }));

    expect(lastCallUrl()).toContain('action=sync');
    expect(lastCallUrl()).not.toContain('confirm=');
    expect(await screen.findByText('Sync requested.')).toBeInTheDocument();
    expect(onDone).toHaveBeenCalled();
  });

  it('refreshes without asking anything', async () => {
    const user = userEvent.setup();
    renderActions();

    await user.click(screen.getByRole('button', { name: 'Refresh' }));

    expect(lastCallUrl()).toContain('action=refresh');
    expect(await screen.findByText('Refresh requested.')).toBeInTheDocument();
  });

  it('reports what the server said when the action fails', async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 404,
        json: () => Promise.resolve({ message: 'applications "podinfo" not found' }),
      }),
    );
    renderActions();

    await user.click(screen.getByRole('button', { name: 'Refresh' }));

    expect(await screen.findByText('applications "podinfo" not found')).toBeInTheDocument();
  });
});

describe('the argo cd actions on a protected cluster', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({ action: 'sync' }) }),
    );
    showModal.mockClear();
    close.mockClear();
    HTMLDialogElement.prototype.showModal = showModal;
    HTMLDialogElement.prototype.close = close;
    protectedCluster();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    openCluster();
  });

  it('asks for the application name before syncing', async () => {
    const user = userEvent.setup();
    renderActions();

    await user.click(screen.getByRole('button', { name: 'Sync' }));

    expect(
      screen.getByText('Syncing Application podinfo against its repository.'),
    ).toBeInTheDocument();
    expect(globalThis.fetch).not.toHaveBeenCalled();
  });

  it('syncs with the confirmation once it is given', async () => {
    const user = userEvent.setup();
    renderActions();
    await user.click(screen.getByRole('button', { name: 'Sync' }));

    await user.type(screen.getByLabelText('Name'), 'podinfo');
    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    expect(lastCallUrl()).toContain('confirm=podinfo');
  });

  it('drops the question when it is cancelled', async () => {
    const user = userEvent.setup();
    renderActions();
    await user.click(screen.getByRole('button', { name: 'Sync' }));

    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(
      screen.queryByText('Syncing Application podinfo against its repository.'),
    ).not.toBeInTheDocument();
    expect(globalThis.fetch).not.toHaveBeenCalled();
  });

  it('still refreshes on one click', async () => {
    const user = userEvent.setup();
    renderActions();

    await user.click(screen.getByRole('button', { name: 'Refresh' }));

    expect(lastCallUrl()).toContain('action=refresh');
    expect(lastCallUrl()).toContain('confirm=podinfo');
  });
});

describe('an argo action that outlives its panel', () => {
  beforeEach(() => {
    showModal.mockClear();
    close.mockClear();
    HTMLDialogElement.prototype.showModal = showModal;
    HTMLDialogElement.prototype.close = close;
    openCluster();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('falls back to a plain message for a rejection that is not an Error', async () => {
    const user = userEvent.setup();
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('nope'));
    renderActions();

    await user.click(screen.getByRole('button', { name: 'Refresh' }));

    expect(await screen.findByText('action failed')).toBeInTheDocument();
  });

  it('drops a success that lands after the panel moved on', async () => {
    const user = userEvent.setup();
    const deferred = {
      release: () => {
        return undefined;
      },
    };
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise((resolve) => {
            deferred.release = () => {
              resolve({ ok: true, json: () => Promise.resolve({ action: 'refresh' }) });
            };
          }),
      ),
    );
    const onDone = vi.fn();
    const view = render(<ArgoActions target={target} onDone={onDone} />);
    await user.click(screen.getByRole('button', { name: 'Refresh' }));
    await screen.findByText('working');

    view.rerender(<ArgoActions target={{ ...target, name: 'other' }} onDone={onDone} />);
    deferred.release();
    await new Promise((resolve) => setTimeout(resolve, 50));

    expect(screen.queryByText('Refresh requested.')).not.toBeInTheDocument();
    expect(onDone).not.toHaveBeenCalled();
  });

  it('drops a failure that lands after the panel moved on', async () => {
    const user = userEvent.setup();
    const deferred = {
      reject: () => {
        return undefined;
      },
    };
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise((_resolve, reject) => {
            deferred.reject = () => {
              reject(new Error('forbidden'));
            };
          }),
      ),
    );
    const view = render(<ArgoActions target={target} onDone={vi.fn()} />);
    await user.click(screen.getByRole('button', { name: 'Refresh' }));
    await screen.findByText('working');

    view.rerender(<ArgoActions target={{ ...target, name: 'other' }} onDone={vi.fn()} />);
    deferred.reject();
    await new Promise((resolve) => setTimeout(resolve, 50));

    expect(screen.queryByText('forbidden')).not.toBeInTheDocument();
  });
});

describe('argo buttons the cluster would refuse', () => {
  beforeEach(() => {
    useAccessStore.getState().forget();
    useContextsStore.getState().setList({
      ...EMPTY_CONTEXTS,
      current: { kubeconfig: '', name: 'p-mk1' },
    });
  });

  afterEach(() => {
    useAccessStore.getState().forget();
  });

  it("greys out Sync and Refresh with the cluster's reason", () => {
    useAccessStore
      .getState()
      .setRefused(accessKey('p-mk1', target), { reconcile: 'no patching applications' });
    renderActions();

    expect(screen.getByRole('button', { name: 'Sync' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Refresh' })).toHaveAttribute(
      'title',
      'no patching applications',
    );
  });

  it('leaves them alone when nothing stands in the way', () => {
    useAccessStore.getState().setRefused(accessKey('p-mk1', target), {});
    renderActions();

    expect(screen.getByRole('button', { name: 'Sync' })).toBeEnabled();
  });
});

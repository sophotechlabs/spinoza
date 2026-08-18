import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import BulkBar from '../../src/components/BulkBar';
import type { ObjectRef } from '../../src/lib/types';
import { useToastsStore } from '../../src/store/toasts';
import { useContextsStore } from '../../src/store/contexts';

function podRef(name: string): ObjectRef {
  return { group: '', version: 'v1', resource: 'pods', namespace: 'prod', name };
}

function deploymentRef(name: string): ObjectRef {
  return { group: 'apps', version: 'v1', resource: 'deployments', namespace: 'prod', name };
}

function renderBar(targets: ObjectRef[], kind = 'Pod') {
  const onDone = vi.fn();
  const onClear = vi.fn();
  const view = render(<BulkBar kind={kind} targets={targets} onDone={onDone} onClear={onClear} />);
  return { onDone, onClear, view };
}

function protectCluster() {
  useContextsStore.getState().setList({
    current: { kubeconfig: '', name: 'p-mk1' },
    kubeconfigs: [],
    protection: 'protected',
  });
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
  useToastsStore.getState().clear();
});

describe('BulkBar', () => {
  it('stays out of the way with nothing selected', () => {
    const { view } = renderBar([]);

    expect(view.container).toBeEmptyDOMElement();
  });

  it('counts one selected object in the singular', () => {
    renderBar([podRef('web-0')]);

    expect(screen.getByRole('status')).toHaveTextContent('1 Pod selected');
  });

  it('counts several', () => {
    renderBar([podRef('web-0'), podRef('web-1')]);

    expect(screen.getByRole('status')).toHaveTextContent('2 Pod objects selected');
  });

  it('offers Restart only for a kind that can be restarted', () => {
    renderBar([podRef('web-0')]);
    expect(screen.queryByRole('button', { name: 'Restart' })).not.toBeInTheDocument();

    renderBar([deploymentRef('web')], 'Deployment');
    expect(screen.getByRole('button', { name: 'Restart' })).toBeInTheDocument();
  });

  it('asks before deleting', async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({}) });
    vi.stubGlobal('fetch', fetchMock);
    renderBar([podRef('web-0')]);

    await user.click(screen.getByRole('button', { name: 'Delete' }));

    expect(screen.getByText('Delete 1 object?')).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('backs out of a delete', async () => {
    const user = userEvent.setup();
    renderBar([podRef('web-0')]);
    await user.click(screen.getByRole('button', { name: 'Delete' }));

    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(screen.getByRole('button', { name: 'Delete' })).toBeInTheDocument();
  });

  it('deletes every selected object and says how many', async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({}) });
    vi.stubGlobal('fetch', fetchMock);
    const { onDone } = renderBar([podRef('web-0'), podRef('web-1')]);

    await user.click(screen.getByRole('button', { name: 'Delete' }));
    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(useToastsStore.getState().toasts).toEqual([
      expect.objectContaining({ tone: 'ok', message: 'Deleted 2' }),
    ]);
    expect(onDone).toHaveBeenCalledTimes(1);
  });

  it('names the ones that failed', async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('web-1')) {
          return Promise.resolve({ ok: false, status: 500, json: () => Promise.resolve({}) });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      }),
    );
    renderBar([podRef('web-0'), podRef('web-1')]);

    await user.click(screen.getByRole('button', { name: 'Delete' }));
    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    expect(useToastsStore.getState().toasts).toEqual([
      expect.objectContaining({ tone: 'error', message: 'Deleted 1, 1 failed: web-1' }),
    ]);
  });

  it('restarts every selected workload', async () => {
    const user = userEvent.setup();
    const fetchMock = vi
      .fn()
      .mockResolvedValue({ ok: true, json: () => Promise.resolve({ action: 'restart' }) });
    vi.stubGlobal('fetch', fetchMock);
    renderBar([deploymentRef('web'), deploymentRef('api')], 'Deployment');

    await user.click(screen.getByRole('button', { name: 'Restart' }));
    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(useToastsStore.getState().toasts).toEqual([
      expect.objectContaining({ tone: 'ok', message: 'Restarted 2' }),
    ]);
  });

  it('hands the selection back when asked to clear it', async () => {
    const user = userEvent.setup();
    const { onClear } = renderBar([podRef('web-0')]);

    await user.click(screen.getByRole('button', { name: 'Clear selection' }));

    expect(onClear).toHaveBeenCalledTimes(1);
  });
});

describe('BulkBar on a protected cluster', () => {
  it('asks for the cluster name before deleting', async () => {
    const user = userEvent.setup();
    protectCluster();
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve({ ok: true, json: () => Promise.resolve({}) })),
    );

    renderBar([podRef('web-0'), podRef('web-1')]);
    await user.click(screen.getByRole('button', { name: 'Delete' }));

    expect(
      screen.getByText(
        'Deleting 2 objects on p-mk1 in one go — this asks for the cluster name, not an object name.',
      ),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeDisabled();
  });

  it('sends each object name once the cluster name is typed', async () => {
    const user = userEvent.setup();
    protectCluster();
    const fetchMock = vi.fn((url: string) => {
      void url;
      return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
    });
    vi.stubGlobal('fetch', fetchMock);

    const { onDone } = renderBar([podRef('web-0'), podRef('web-1')]);
    await user.click(screen.getByRole('button', { name: 'Delete' }));
    await user.type(screen.getByLabelText('Name'), 'p-mk1');
    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    await waitFor(() => {
      expect(onDone).toHaveBeenCalled();
    });
    expect(fetchMock.mock.calls[0][0]).toContain('confirm=web-0');
    expect(fetchMock.mock.calls[1][0]).toContain('confirm=web-1');
  });

  it('drops the question when it is cancelled', async () => {
    const user = userEvent.setup();
    protectCluster();
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve({ ok: true, json: () => Promise.resolve({}) })),
    );

    renderBar([podRef('web-0')]);
    await user.click(screen.getByRole('button', { name: 'Delete' }));
    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(screen.queryByLabelText('Name')).not.toBeInTheDocument();
  });

  it('still restarts with one click', async () => {
    const user = userEvent.setup();
    protectCluster();
    const fetchMock = vi.fn((url: string) => {
      void url;
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ action: 'restart', message: 'ok' }),
      });
    });
    vi.stubGlobal('fetch', fetchMock);

    const { onDone } = renderBar([deploymentRef('web')], 'Deployment');
    await user.click(screen.getByRole('button', { name: 'Restart' }));
    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    await waitFor(() => {
      expect(onDone).toHaveBeenCalled();
    });
    expect(fetchMock.mock.calls[0][0]).not.toContain('confirm');
  });
});

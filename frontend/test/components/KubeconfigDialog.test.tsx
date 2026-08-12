import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import KubeconfigDialog from '../../src/components/KubeconfigDialog';
import type { Kubeconfig } from '../../src/lib/types';
import { useToastsStore } from '../../src/store/toasts';

const showModal = vi.fn(function showModal(this: HTMLDialogElement) {
  this.open = true;
});
const close = vi.fn(function close(this: HTMLDialogElement) {
  this.open = false;
});

const fallback: Kubeconfig = {
  label: '/home/arch/.kube/config',
  path: '',
  removable: false,
  contexts: [
    { name: 'p-mk1', cluster: 'p-mk1' },
    { name: 'p-mk2', cluster: 'p-mk2' },
  ],
};

const work: Kubeconfig = {
  label: '/home/arch/.kube/work.yaml',
  path: '/home/arch/.kube/work.yaml',
  removable: true,
  contexts: [{ name: 'staging', cluster: 'work' }],
};

interface Reply {
  ok: boolean;
  status?: number;
  body: unknown;
}

function stubFetch(replies: { picker?: Reply; change?: Reply; pick?: Reply }) {
  const calls: { url: string; method?: string }[] = [];
  const answer = (reply: Reply | undefined) => {
    const given = reply ?? { ok: true, body: {} };
    return Promise.resolve({
      ok: given.ok,
      status: given.status ?? 200,
      json: () => Promise.resolve(given.body),
    });
  };
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: { method?: string }) => {
      calls.push({ url, method: init?.method });
      if (url.startsWith('/api/kubeconfigs/picker')) {
        if (init?.method === 'POST') {
          return answer(replies.pick);
        }
        return answer(replies.picker);
      }
      return answer(replies.change);
    }),
  );
  return calls;
}

function open(kubeconfigs: Kubeconfig[] = [fallback, work]) {
  const onChanged = vi.fn();
  const onClose = vi.fn();
  render(
    <KubeconfigDialog open kubeconfigs={kubeconfigs} onChanged={onChanged} onClose={onClose} />,
  );
  return { onChanged, onClose };
}

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

describe('KubeconfigDialog', () => {
  it('lists every kubeconfig spinoza reads', () => {
    stubFetch({});

    open();

    expect(screen.getByText('/home/arch/.kube/config')).toBeInTheDocument();
    expect(screen.getByText(/read by default/)).toBeInTheDocument();
    expect(screen.getByText('2 contexts')).toBeInTheDocument();
    expect(screen.getByText('1 context')).toBeInTheDocument();
  });

  it('offers removal only for the kubeconfigs that were added', () => {
    stubFetch({});

    open();

    expect(screen.getByRole('button', { name: 'Remove /home/arch/.kube/work.yaml' })).toBeVisible();
    expect(
      screen.queryByRole('button', { name: 'Remove /home/arch/.kube/config' }),
    ).not.toBeInTheDocument();
  });

  it('shows why a kubeconfig could not be read', () => {
    stubFetch({});

    open([{ ...work, contexts: [], error: 'kubeconfig: no such file or directory' }]);

    expect(screen.getByText('kubeconfig: no such file or directory')).toBeInTheDocument();
    expect(screen.getByText('0 contexts')).toBeInTheDocument();
  });

  it('opens itself when asked to', () => {
    stubFetch({});

    open();

    expect(showModal).toHaveBeenCalled();
  });

  it('closes again when it is no longer open', () => {
    stubFetch({});
    const view = render(
      <KubeconfigDialog open kubeconfigs={[fallback]} onChanged={vi.fn()} onClose={vi.fn()} />,
    );

    view.rerender(
      <KubeconfigDialog
        open={false}
        kubeconfigs={[fallback]}
        onChanged={vi.fn()}
        onClose={vi.fn()}
      />,
    );

    expect(close).toHaveBeenCalled();
  });

  it('adds the kubeconfig at the path that was typed', async () => {
    const user = userEvent.setup();
    const calls = stubFetch({
      change: { ok: true, body: { current: { kubeconfig: '', name: 'p-mk1' }, kubeconfigs: [] } },
    });
    const { onChanged } = open([fallback]);

    await user.type(screen.getByLabelText('Add a kubeconfig'), '~/.kube/work.yaml');
    await user.click(screen.getByRole('button', { name: 'Add' }));

    await waitFor(() => {
      expect(onChanged).toHaveBeenCalled();
    });
    const post = calls.find((call) => call.method === 'POST' && !call.url.includes('picker'));
    expect(post?.url).toContain('path=%7E%2F.kube%2Fwork.yaml');
    expect(useToastsStore.getState().toasts).toEqual([
      expect.objectContaining({ tone: 'ok', message: 'Reading ~/.kube/work.yaml' }),
    ]);
  });

  it('clears the path once the kubeconfig is added', async () => {
    const user = userEvent.setup();
    stubFetch({
      change: { ok: true, body: { current: { kubeconfig: '', name: 'p-mk1' }, kubeconfigs: [] } },
    });
    open([fallback]);

    await user.type(screen.getByLabelText('Add a kubeconfig'), '/tmp/work.yaml');
    await user.click(screen.getByRole('button', { name: 'Add' }));

    await waitFor(() => {
      expect(screen.getByLabelText('Add a kubeconfig')).toHaveValue('');
    });
  });

  it('asks for a path before calling the backend', async () => {
    const user = userEvent.setup();
    const calls = stubFetch({});
    open([fallback]);

    await user.click(screen.getByRole('button', { name: 'Add' }));

    expect(await screen.findByText('type the path of a kubeconfig file')).toBeInTheDocument();
    expect(calls.filter((call) => call.method === 'POST')).toHaveLength(0);
  });

  it('reports a kubeconfig the backend refused', async () => {
    const user = userEvent.setup();
    stubFetch({
      change: { ok: false, status: 400, body: { message: 'that file is not a kubeconfig' } },
    });
    open([fallback]);

    await user.type(screen.getByLabelText('Add a kubeconfig'), '/tmp/notes.txt');
    await user.click(screen.getByRole('button', { name: 'Add' }));

    expect(await screen.findByText('that file is not a kubeconfig')).toBeInTheDocument();
    expect(useToastsStore.getState().toasts).toEqual([
      expect.objectContaining({
        tone: 'error',
        message: 'Adding that kubeconfig: that file is not a kubeconfig',
      }),
    ]);
  });

  it('falls back to a generic message for a non-Error rejection', async () => {
    const user = userEvent.setup();
    const rejectNonError = vi.fn<() => Promise<never>>().mockRejectedValue('nope');
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.startsWith('/api/kubeconfigs/picker')) {
          return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: false }) });
        }
        return rejectNonError();
      }),
    );
    open([fallback]);

    await user.type(screen.getByLabelText('Add a kubeconfig'), '/tmp/work.yaml');
    await user.click(screen.getByRole('button', { name: 'Add' }));

    expect(await screen.findByText('Adding that kubeconfig')).toBeInTheDocument();
  });

  it('removes the kubeconfig it was asked to drop', async () => {
    const user = userEvent.setup();
    const calls = stubFetch({
      change: { ok: true, body: { current: { kubeconfig: '', name: 'p-mk1' }, kubeconfigs: [] } },
    });
    const { onChanged } = open();

    await user.click(screen.getByRole('button', { name: 'Remove /home/arch/.kube/work.yaml' }));

    await waitFor(() => {
      expect(onChanged).toHaveBeenCalled();
    });
    const deleted = calls.find((call) => call.method === 'DELETE');
    expect(deleted?.url).toContain('path=%2Fhome%2Farch%2F.kube%2Fwork.yaml');
    expect(useToastsStore.getState().toasts).toEqual([
      expect.objectContaining({
        tone: 'ok',
        message: 'Stopped reading /home/arch/.kube/work.yaml',
      }),
    ]);
  });

  it('reports a removal the backend refused', async () => {
    const user = userEvent.setup();
    stubFetch({
      change: {
        ok: false,
        status: 400,
        body: { message: 'spinoza is connected through that kubeconfig' },
      },
    });
    open();

    await user.click(screen.getByRole('button', { name: 'Remove /home/arch/.kube/work.yaml' }));

    expect(
      await screen.findByText('spinoza is connected through that kubeconfig'),
    ).toBeInTheDocument();
  });

  it('offers no file dialog in a browser tab', async () => {
    stubFetch({
      picker: { ok: true, body: { available: false, reason: 'only the desktop window' } },
    });

    open();

    await waitFor(() => {
      expect(screen.queryByRole('button', { name: 'Browse…' })).not.toBeInTheDocument();
    });
  });

  it('fills the path from the desktop file dialog', async () => {
    const user = userEvent.setup();
    stubFetch({
      picker: { ok: true, body: { available: true } },
      pick: { ok: true, body: { path: '/home/arch/.kube/work.yaml' } },
    });
    open([fallback]);

    await user.click(await screen.findByRole('button', { name: 'Browse…' }));

    await waitFor(() => {
      expect(screen.getByLabelText('Add a kubeconfig')).toHaveValue('/home/arch/.kube/work.yaml');
    });
  });

  it('leaves the path alone when the file dialog was cancelled', async () => {
    const user = userEvent.setup();
    stubFetch({
      picker: { ok: true, body: { available: true } },
      pick: { ok: true, body: { path: '' } },
    });
    open([fallback]);
    await user.type(screen.getByLabelText('Add a kubeconfig'), '/tmp/typed.yaml');

    await user.click(await screen.findByRole('button', { name: 'Browse…' }));

    await waitFor(() => {
      expect(screen.getByLabelText('Add a kubeconfig')).toHaveValue('/tmp/typed.yaml');
    });
  });

  it('reports a file dialog that did not open', async () => {
    const user = userEvent.setup();
    stubFetch({
      picker: { ok: true, body: { available: true } },
      pick: { ok: false, status: 501, body: { message: 'the spinoza window is not ready yet' } },
    });
    open([fallback]);

    await user.click(await screen.findByRole('button', { name: 'Browse…' }));

    expect(await screen.findByText('the spinoza window is not ready yet')).toBeInTheDocument();
  });

  it('keeps quiet when the picker support call fails', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));

    open([fallback]);

    await waitFor(() => {
      expect(screen.queryByRole('button', { name: 'Browse…' })).not.toBeInTheDocument();
    });
  });

  it('asks for nothing while it is closed', () => {
    const calls = stubFetch({});

    render(
      <KubeconfigDialog
        open={false}
        kubeconfigs={[fallback]}
        onChanged={vi.fn()}
        onClose={vi.fn()}
      />,
    );

    expect(calls).toHaveLength(0);
  });

  it('drops picker support that lands after it closed', async () => {
    const deferred: { settle: () => void } = {
      settle: () => {
        return undefined;
      },
    };
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise((resolve) => {
            deferred.settle = () => {
              resolve({ ok: true, json: () => Promise.resolve({ available: true }) });
            };
          }),
      ),
    );
    const view = render(
      <KubeconfigDialog open kubeconfigs={[fallback]} onChanged={vi.fn()} onClose={vi.fn()} />,
    );

    view.unmount();
    deferred.settle();
    await Promise.resolve();

    expect(screen.queryByRole('button', { name: 'Browse…' })).not.toBeInTheDocument();
  });

  it('closes on the close button', async () => {
    const user = userEvent.setup();
    stubFetch({});
    const { onClose } = open();

    await user.click(screen.getByRole('button', { name: 'Close' }));

    expect(onClose).toHaveBeenCalled();
  });
});

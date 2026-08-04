import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ContextPicker from '../../src/components/ContextPicker';
import { useToastsStore } from '../../src/store/toasts';

function stubContexts(list: unknown, post?: unknown) {
  const calls: { url: string; method?: string }[] = [];
  const fetchMock = vi.fn((url: string, init?: { method?: string }) => {
    calls.push({ url, method: init?.method });
    if (init?.method === 'POST') {
      if (post === undefined) {
        return Promise.resolve({
          ok: false,
          status: 400,
          json: () => Promise.resolve({ message: 'context "gone" does not exist' }),
        });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve(post) });
    }
    return Promise.resolve({ ok: true, json: () => Promise.resolve(list) });
  });
  vi.stubGlobal('fetch', fetchMock);
  return calls;
}

beforeEach(() => {
  useToastsStore.getState().clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
  useToastsStore.getState().clear();
});

describe('ContextPicker', () => {
  it('lists every context with the current one selected', async () => {
    stubContexts({ contexts: ['p-mk1', 'p-mk2'], current: 'p-mk2' });

    render(<ContextPicker onSwitched={vi.fn()} />);

    const picker = await screen.findByLabelText('Kubernetes context');
    expect(picker).toHaveValue('p-mk2');
    expect(screen.getByRole('option', { name: 'p-mk1' })).toBeInTheDocument();
  });

  it('switches to the chosen context and tells the app to reconnect', async () => {
    const user = userEvent.setup();
    const calls = stubContexts(
      { contexts: ['p-mk1', 'p-mk2'], current: 'p-mk2' },
      { contexts: ['p-mk1', 'p-mk2'], current: 'p-mk1' },
    );
    const onSwitched = vi.fn();
    render(<ContextPicker onSwitched={onSwitched} />);
    await screen.findByLabelText('Kubernetes context');

    await user.selectOptions(screen.getByLabelText('Kubernetes context'), 'p-mk1');

    await waitFor(() => {
      expect(onSwitched).toHaveBeenCalled();
    });
    const post = calls.find((call) => call.method === 'POST');
    expect(post?.url).toContain('name=p-mk1');
    expect(screen.getByLabelText('Kubernetes context')).toHaveValue('p-mk1');
    expect(useToastsStore.getState().toasts).toEqual([
      expect.objectContaining({ tone: 'ok', message: 'Switched to p-mk1' }),
    ]);
  });

  it('reports a switch the server refused and keeps the old context', async () => {
    const user = userEvent.setup();
    stubContexts({ contexts: ['p-mk1', 'p-mk2'], current: 'p-mk2' });
    const onSwitched = vi.fn();
    render(<ContextPicker onSwitched={onSwitched} />);
    await screen.findByLabelText('Kubernetes context');

    await user.selectOptions(screen.getByLabelText('Kubernetes context'), 'p-mk1');

    expect(await screen.findByText('context "gone" does not exist')).toBeInTheDocument();
    expect(onSwitched).not.toHaveBeenCalled();
    expect(useToastsStore.getState().toasts).toEqual([
      expect.objectContaining({
        tone: 'error',
        message: 'Switching to p-mk1: context "gone" does not exist',
      }),
    ]);
  });

  it('shows a plain label when the kubeconfig has no contexts', async () => {
    stubContexts({ contexts: [], current: 'embedded' });

    render(<ContextPicker onSwitched={vi.fn()} />);

    expect(await screen.findByText('embedded')).toBeInTheDocument();
    expect(screen.queryByLabelText('Kubernetes context')).not.toBeInTheDocument();
  });

  it('reports a listing the server refused instead of an empty header slot', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 500,
        json: () => Promise.resolve({ message: 'kubeconfig is unreadable' }),
      }),
    );

    render(<ContextPicker onSwitched={vi.fn()} />);

    expect(await screen.findByText(/kubeconfig is unreadable/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument();
    expect(screen.queryByLabelText('Kubernetes context')).not.toBeInTheDocument();
  });

  it('recovers from the retry button once the backend answers', async () => {
    const user = userEvent.setup();
    const fetchMock = vi
      .fn()
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ contexts: ['p-mk1', 'p-mk2'], current: 'p-mk1' }),
      });
    vi.stubGlobal('fetch', fetchMock);
    render(<ContextPicker onSwitched={vi.fn()} />);
    await screen.findByRole('button', { name: 'Retry' });

    await user.click(screen.getByRole('button', { name: 'Retry' }));

    expect(await screen.findByLabelText('Kubernetes context')).toHaveValue('p-mk1');
  });

  it('keeps retrying on its own until the backend comes back', async () => {
    const fetchMock = vi
      .fn()
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ contexts: ['p-mk1'], current: 'p-mk1' }),
      });
    vi.stubGlobal('fetch', fetchMock);
    render(<ContextPicker onSwitched={vi.fn()} />);
    await screen.findByRole('button', { name: 'Retry' });

    expect(await screen.findByLabelText('Kubernetes context')).toBeInTheDocument();
  });

  it('surfaces the reason the kubeconfig produced no contexts', async () => {
    stubContexts({ contexts: [], current: '', error: 'kubeconfig has no current-context' });

    render(<ContextPicker onSwitched={vi.fn()} />);

    expect(await screen.findByText(/kubeconfig has no current-context/)).toBeInTheDocument();
  });

  it('falls back to a generic message for a non-Error rejection', async () => {
    const user = userEvent.setup();
    const rejectNonError = vi.fn<() => Promise<never>>().mockRejectedValue('nope');
    const fetchMock = vi.fn((_url: string, init?: { method?: string }) => {
      if (init?.method === 'POST') {
        return rejectNonError();
      }
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ contexts: ['p-mk1', 'p-mk2'], current: 'p-mk2' }),
      });
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<ContextPicker onSwitched={vi.fn()} />);
    await screen.findByLabelText('Kubernetes context');

    await user.selectOptions(screen.getByLabelText('Kubernetes context'), 'p-mk1');

    expect(await screen.findByText('switching context failed')).toBeInTheDocument();
  });

  it('drops a listing that lands after unmount', () => {
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
              resolve({
                ok: true,
                json: () => Promise.resolve({ contexts: ['p-mk1'], current: 'p-mk1' }),
              });
            };
          }),
      ),
    );
    const view = render(<ContextPicker onSwitched={vi.fn()} />);

    view.unmount();
    deferred.settle();

    expect(screen.queryByLabelText('Kubernetes context')).not.toBeInTheDocument();
  });

  it('drops a listing failure that lands after unmount', async () => {
    const deferred = {
      fail: () => {
        return undefined;
      },
    };
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise((_resolve, reject) => {
            deferred.fail = () => {
              reject(new Error('too late'));
            };
          }),
      ),
    );
    const view = render(<ContextPicker onSwitched={vi.fn()} />);

    view.unmount();
    deferred.fail();
    await Promise.resolve();

    expect(screen.queryByText(/too late/)).not.toBeInTheDocument();
  });

  it('ignores re-picking the context already in use', async () => {
    const user = userEvent.setup();
    const calls = stubContexts({ contexts: ['p-mk1', 'p-mk2'], current: 'p-mk2' });
    render(<ContextPicker onSwitched={vi.fn()} />);
    await screen.findByLabelText('Kubernetes context');

    await user.selectOptions(screen.getByLabelText('Kubernetes context'), 'p-mk2');

    expect(calls.filter((call) => call.method === 'POST')).toHaveLength(0);
  });

  it('survives a response with no contexts field', async () => {
    stubContexts({});

    render(<ContextPicker onSwitched={vi.fn()} />);

    await waitFor(() => {
      expect(screen.queryByLabelText('Kubernetes context')).not.toBeInTheDocument();
    });
  });

  it('names a contexts request that failed outright', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));

    render(<ContextPicker onSwitched={vi.fn()} />);

    expect(await screen.findByText(/offline/)).toBeInTheDocument();
  });

  it('falls back to a generic message when the listing rejects with a non-Error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('nope'));

    render(<ContextPicker onSwitched={vi.fn()} />);

    expect(await screen.findByText(/the context list could not be loaded/)).toBeInTheDocument();
  });
});

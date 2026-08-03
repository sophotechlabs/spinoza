import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ContextPicker from '../../src/components/ContextPicker';

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

afterEach(() => {
  vi.unstubAllGlobals();
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
  });

  it('shows a plain label when the kubeconfig has no contexts', async () => {
    stubContexts({ contexts: [], current: 'embedded' });

    render(<ContextPicker onSwitched={vi.fn()} />);

    expect(await screen.findByText('embedded')).toBeInTheDocument();
    expect(screen.queryByLabelText('Kubernetes context')).not.toBeInTheDocument();
  });

  it('reports a listing the server refused', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 500,
        json: () => Promise.resolve({ message: 'kubeconfig is unreadable' }),
      }),
    );

    render(<ContextPicker onSwitched={vi.fn()} />);

    await waitFor(() => {
      expect(screen.queryByLabelText('Kubernetes context')).not.toBeInTheDocument();
    });
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

  it('survives a contexts request that fails', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));

    render(<ContextPicker onSwitched={vi.fn()} />);

    await waitFor(() => {
      expect(screen.queryByLabelText('Kubernetes context')).not.toBeInTheDocument();
    });
  });
});

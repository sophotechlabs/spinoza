import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ForwardsPanel from '../../src/components/ForwardsPanel';
import { setForwards } from '../../src/store/forwards';
import { useToastsStore } from '../../src/store/toasts';
import type { PortForward } from '../../src/lib/types';
import { anySignal, rejectsWith } from '../helpers';
import { adoptSession } from '../../src/store/identity';
import { OWN_WINDOW } from '../../src/lib/identity';
import { bumpClusterEpoch } from '../../src/store/cluster';

function forward(overrides: Partial<PortForward> = {}): PortForward {
  return {
    id: 'pf-1',
    kind: 'Pod',
    namespace: 'flux-system',
    name: 'web',
    remotePort: 8080,
    localPort: 45123,
    state: 'running',
    startedAt: '2026-07-27T18:00:00Z',
    ...overrides,
  };
}

function stubList(forwards: PortForward[]) {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(forwards) }),
  );
}

describe('ForwardsPanel', () => {
  beforeEach(() => {
    setForwards([]);
    useToastsStore.getState().clear();
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve([]) }),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
    setForwards([]);
  });

  it('prompts when there is nothing forwarded', () => {
    render(<ForwardsPanel />);

    expect(screen.getByText(/No active forwards/)).toBeInTheDocument();
  });

  it('lists a running forward with a clickable local address', async () => {
    stubList([forward()]);
    render(<ForwardsPanel />);

    const link = await screen.findByRole('link', { name: '127.0.0.1:45123' });
    expect(link).toHaveAttribute('href', 'http://127.0.0.1:45123');
    expect(screen.getByText('pod/flux-system/web')).toBeInTheDocument();
    expect(screen.getByText('to 8080')).toBeInTheDocument();
  });

  it('opens the local address in a browser instead of the panel', async () => {
    const user = userEvent.setup();
    stubList([forward()]);
    const opened = vi.fn();
    vi.stubGlobal('open', opened);
    render(<ForwardsPanel />);

    await user.click(await screen.findByRole('link', { name: '127.0.0.1:45123' }));

    expect(opened).toHaveBeenCalledWith('http://127.0.0.1:45123', '_blank', 'noreferrer');
  });

  it('shows the error for a failed forward instead of a link', async () => {
    stubList([forward({ state: 'failed', error: 'pod deleted' })]);
    render(<ForwardsPanel />);

    expect(await screen.findByText('pod deleted')).toBeInTheDocument();
    expect(screen.queryByRole('link')).not.toBeInTheDocument();
  });

  it('stops a forward', async () => {
    const user = userEvent.setup();
    const mock = vi.fn().mockImplementation((_url: string, init?: RequestInit) => {
      if (init?.method === 'DELETE') {
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve([forward()]) });
    });
    vi.stubGlobal('fetch', mock);
    render(<ForwardsPanel />);

    await user.click(await screen.findByRole('button', { name: 'Stop' }));

    expect(mock).toHaveBeenCalledWith('/api/portforward?id=pf-1', {
      method: 'DELETE',
      signal: anySignal(),
    });
    expect(useToastsStore.getState().toasts).toEqual([
      expect.objectContaining({ tone: 'ok', message: 'Forward stopped' }),
    ]);
  });

  it('reports a failure to stop', async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((_url: string, init?: RequestInit) => {
        if (init?.method === 'DELETE') {
          return Promise.resolve({ ok: false, status: 404, json: () => Promise.resolve({}) });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve([forward()]) });
      }),
    );
    render(<ForwardsPanel />);

    await user.click(await screen.findByRole('button', { name: 'Stop' }));

    expect(
      await screen.findByText('stopping the forward failed with status 404'),
    ).toBeInTheDocument();
  });

  it('drops a stop completion after the cluster reconnects', async () => {
    const user = userEvent.setup();
    let finish!: (response: { ok: boolean; json: () => Promise<unknown> }) => void;
    const stopping = new Promise<{ ok: boolean; json: () => Promise<unknown> }>((resolve) => {
      finish = resolve;
    });
    vi.stubGlobal(
      'fetch',
      vi.fn((_url: string, init?: RequestInit) => {
        if (init?.method === 'DELETE') {
          return stopping;
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve([forward()]) });
      }),
    );
    render(<ForwardsPanel />);
    await user.click(await screen.findByRole('button', { name: 'Stop' }));

    act(() => {
      bumpClusterEpoch();
    });
    await act(async () => {
      finish({ ok: true, json: () => Promise.resolve({}) });
      await stopping;
      await Promise.resolve();
    });

    expect(useToastsStore.getState().toasts).toHaveLength(0);
  });

  it('drops a stop failure after the cluster reconnects', async () => {
    const user = userEvent.setup();
    let fail!: (reason: unknown) => void;
    const stopping = new Promise((_resolve, reject) => {
      fail = reject;
    });
    vi.stubGlobal(
      'fetch',
      vi.fn((_url: string, init?: RequestInit) => {
        if (init?.method === 'DELETE') {
          return stopping;
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve([forward()]) });
      }),
    );
    render(<ForwardsPanel />);
    await user.click(await screen.findByRole('button', { name: 'Stop' }));

    act(() => {
      bumpClusterEpoch();
    });
    await act(async () => {
      fail(new Error('old cluster failed'));
      await Promise.resolve();
    });

    expect(useToastsStore.getState().toasts).toHaveLength(0);
  });

  it('falls back to a generic message for a non-Error rejection', async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((_url: string, init?: RequestInit) => {
        if (init?.method === 'DELETE') {
          return rejectsWith('nope')();
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve([forward()]) });
      }),
    );
    render(<ForwardsPanel />);

    await user.click(await screen.findByRole('button', { name: 'Stop' }));

    expect(await screen.findByText('could not stop the forward')).toBeInTheDocument();
  });

  it('keeps the server message instead of a fixed string', async () => {
    const user = userEvent.setup();
    useToastsStore.getState().clear();
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((_url: string, init?: RequestInit) => {
        if (init?.method === 'DELETE') {
          return Promise.resolve({
            ok: false,
            status: 409,
            json: () => Promise.resolve({ message: 'forward pf-1 is already gone' }),
          });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve([forward()]) });
      }),
    );
    render(<ForwardsPanel />);

    await user.click(await screen.findByRole('button', { name: 'Stop' }));

    expect(await screen.findByText('forward pf-1 is already gone')).toBeInTheDocument();
    expect(useToastsStore.getState().toasts).toEqual([
      expect.objectContaining({
        tone: 'error',
        message: 'Stopping the forward: forward pf-1 is already gone',
      }),
    ]);
  });

  it('says the list stopped updating while keeping the forwards on screen', async () => {
    vi.useFakeTimers();
    let call = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(() => {
        call += 1;
        if (call === 1) {
          return Promise.resolve({ ok: true, json: () => Promise.resolve([forward()]) });
        }
        return Promise.reject(new Error('portforward endpoint is down'));
      }),
    );
    render(<ForwardsPanel />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByText('pod/flux-system/web')).toBeInTheDocument();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });

    expect(screen.getByRole('status')).toHaveTextContent('portforward endpoint is down');
    expect(screen.getByText('pod/flux-system/web')).toBeInTheDocument();
  });

  it('says an empty list stopped updating too', async () => {
    vi.useFakeTimers();
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('portforward endpoint is down')));
    render(<ForwardsPanel />);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    expect(screen.getByRole('status')).toHaveTextContent('portforward endpoint is down');
    expect(screen.getByText(/No active forwards/)).toBeInTheDocument();
  });
});

describe('copying a forward url', () => {
  it('offers a copy button next to a running forward', async () => {
    setForwards([forward()]);
    render(<ForwardsPanel />);

    expect(await screen.findByRole('button', { name: 'Copy web forward url' })).toBeInTheDocument();
  });

  it('offers nothing to copy for a failed one', () => {
    setForwards([forward({ state: 'failed', error: 'boom' })]);
    render(<ForwardsPanel />);

    expect(screen.queryByRole('button', { name: /forward url/ })).not.toBeInTheDocument();
  });
});

describe('the forwards panel when spinoza serves the cluster', () => {
  it('says why there is nothing to forward here', () => {
    adoptSession({
      ...OWN_WINDOW,
      cluster: true,
      authenticated: true,
      mode: 'oidc',
      role: 'admin',
    });

    render(<ForwardsPanel />);

    expect(screen.getByText(/would land on the server/)).toBeInTheDocument();
    expect(screen.getByText(/kubectl port-forward/)).toBeInTheDocument();
  });
});

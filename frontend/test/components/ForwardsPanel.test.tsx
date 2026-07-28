import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ForwardsPanel from '../../src/components/ForwardsPanel';
import { useForwardsStore } from '../../src/store/forwards';
import type { PortForward } from '../../src/lib/types';

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
    useForwardsStore.setState({ forwards: [] });
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve([]) }),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    useForwardsStore.setState({ forwards: [] });
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
    expect(screen.getByText('→ 8080')).toBeInTheDocument();
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

    expect(mock).toHaveBeenCalledWith('/api/portforward?id=pf-1', { method: 'DELETE' });
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

    expect(await screen.findByText('could not stop the forward')).toBeInTheDocument();
  });
});

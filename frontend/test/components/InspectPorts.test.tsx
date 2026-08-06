import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import InspectPorts from '../../src/components/InspectPorts';
import { useForwardsStore } from '../../src/store/forwards';
import { useToastsStore } from '../../src/store/toasts';
import type { ObjectPort, ObjectRef } from '../../src/lib/types';

const target: ObjectRef = {
  group: '',
  version: 'v1',
  resource: 'pods',
  namespace: 'flux-system',
  name: 'web',
};

const ports: ObjectPort[] = [{ name: 'http', port: 8080, protocol: 'TCP' }, { port: 9090 }];

function running(localPort: number, remotePort: number) {
  return {
    id: 'pf-1',
    kind: 'Pod',
    namespace: 'flux-system',
    name: 'web',
    pod: 'web',
    remotePort,
    localPort,
    state: 'running',
    startedAt: '2026-08-06T09:00:00Z',
  };
}

function stubExisting(localPort: number) {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      if (url.startsWith('/api/portforward') && init?.method === undefined) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve([running(localPort, 8080)]),
        });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
    }),
  );
}

function stubStart(localPort: number) {
  let started = false;
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      if (url.startsWith('/api/portforward?kind')) {
        started = true;
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve(running(localPort, 8080)),
        });
      }
      if (url.startsWith('/api/portforward?id')) {
        started = false;
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      }
      if (init?.method === undefined) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve(started ? [running(localPort, 8080)] : []),
        });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve([]) });
    }),
  );
}

describe('InspectPorts', () => {
  beforeEach(() => {
    useForwardsStore.setState({ forwards: [] });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    useForwardsStore.setState({ forwards: [] });
    useToastsStore.getState().clear();
  });

  it('labels named and unnamed ports', () => {
    stubStart(45123);
    render(<InspectPorts target={target} kind="Pod" ports={ports} />);

    expect(screen.getByText('8080 · http')).toBeInTheDocument();
    expect(screen.getByText('9090')).toBeInTheDocument();
  });

  it('starts a forward and reports the local address', async () => {
    const user = userEvent.setup();
    stubStart(45123);
    render(<InspectPorts target={target} kind="Pod" ports={ports} />);

    await user.click(screen.getAllByRole('button', { name: 'Forward' })[0]);

    expect(await screen.findByText('127.0.0.1:45123 → 8080')).toBeInTheDocument();
    expect(useToastsStore.getState().toasts).toEqual([
      expect.objectContaining({ tone: 'ok', message: 'Forwarding web 127.0.0.1:45123 → 8080' }),
    ]);
  });

  it('surfaces a failure', async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 400,
        json: () => Promise.resolve({ message: 'no ready pod' }),
      }),
    );
    render(<InspectPorts target={target} kind="Service" ports={ports} />);

    await user.click(screen.getAllByRole('button', { name: 'Forward' })[0]);

    expect(await screen.findByText('no ready pod')).toBeInTheDocument();
    expect(useToastsStore.getState().toasts).toEqual([
      expect.objectContaining({
        tone: 'error',
        message: 'Forwarding web port 8080: no ready pod',
      }),
    ]);
  });

  it('falls back to a generic message for a non-Error rejection', async () => {
    const user = userEvent.setup();
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('nope'));
    render(<InspectPorts target={target} kind="Pod" ports={ports} />);

    await user.click(screen.getAllByRole('button', { name: 'Forward' })[0]);

    expect(await screen.findByText('port forward failed')).toBeInTheDocument();
  });

  it('stops a forward from the port it belongs to', async () => {
    const user = userEvent.setup();
    stubStart(45123);
    render(<InspectPorts target={target} kind="Pod" ports={ports} />);
    await user.click(screen.getAllByRole('button', { name: 'Forward' })[0]);
    expect(await screen.findByText('127.0.0.1:45123 → 8080')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Stop forwarding port 8080' }));

    expect(await screen.findAllByRole('button', { name: 'Forward' })).toHaveLength(2);
    expect(screen.queryByText('127.0.0.1:45123 → 8080')).not.toBeInTheDocument();
  });

  it('shows what another view already started', async () => {
    stubExisting(45123);
    render(<InspectPorts target={target} kind="Pod" ports={ports} />);

    expect(await screen.findByText('127.0.0.1:45123 → 8080')).toBeInTheDocument();
    expect(screen.getAllByRole('button', { name: 'Forward' })).toHaveLength(1);
  });

  it('opens the forwarded address in a browser', async () => {
    const user = userEvent.setup();
    stubExisting(45123);
    const opened = vi.fn();
    vi.stubGlobal('open', opened);
    render(<InspectPorts target={target} kind="Pod" ports={ports} />);
    await screen.findByText('127.0.0.1:45123 → 8080');

    await user.click(screen.getByRole('button', { name: 'Open 127.0.0.1:45123 in a browser' }));

    expect(opened).toHaveBeenCalledWith('http://127.0.0.1:45123', '_blank', 'noreferrer');
  });

  it('leaves a port with no forward alone', async () => {
    stubExisting(45123);
    render(<InspectPorts target={target} kind="Pod" ports={ports} />);
    await screen.findByText('127.0.0.1:45123 → 8080');

    expect(screen.queryByRole('button', { name: 'Stop forwarding port 9090' })).toBeNull();
  });
});

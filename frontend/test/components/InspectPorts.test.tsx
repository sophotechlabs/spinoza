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

function stubStart(localPort: number) {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string) => {
      if (url.startsWith('/api/portforward?kind')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ id: 'pf-1', localPort, state: 'running' }),
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
});

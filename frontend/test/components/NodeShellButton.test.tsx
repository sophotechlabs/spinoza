import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import NodeShellButton from '../../src/components/NodeShellButton';
import { terminalsNow, useTerminalsStore } from '../../src/store/terminals';
import { useSettingsStore } from '../../src/store/settings';
import { bumpClusterEpoch, useClusterStore } from '../../src/store/cluster';

function support(overrides: Record<string, unknown> = {}) {
  return {
    node: 'p-mk1',
    enabled: true,
    allowed: true,
    image: 'busybox:1.37',
    namespace: 'kube-system',
    ...overrides,
  };
}

function stub(body: unknown, ok = true) {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({ ok, status: ok ? 200 : 500, json: () => Promise.resolve(body) }),
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
  useTerminalsStore.getState().reset();
  useSettingsStore.setState({ nodeShell: false });
  act(() => {
    useClusterStore.getState().reset();
  });
});

describe('the node shell button', () => {
  it('stays disabled while the answer is still coming', () => {
    stub(support());

    render(<NodeShellButton node="p-mk1" />);

    expect(screen.getByRole('button', { name: 'Node shell' })).toBeDisabled();
  });

  it('says what it would create before it creates it', async () => {
    stub(support());

    render(<NodeShellButton node="p-mk1" />);

    const explained = await screen.findByTitle(
      'Opens a root shell on p-mk1 by running a privileged busybox:1.37 pod in kube-system',
    );
    expect(explained).toBeInTheDocument();
  });

  it('opens once the cluster allows it', async () => {
    const user = userEvent.setup();
    stub(support());
    render(<NodeShellButton node="p-mk1" />);
    const button = await screen.findByRole('button', { name: 'Node shell' });
    await vi.waitFor(() => {
      expect(button).toBeEnabled();
    });

    await user.click(button);

    const sessions = terminalsNow();
    expect(sessions).toHaveLength(1);
    expect(sessions[0]).toMatchObject({ kind: 'node', pod: 'p-mk1' });
  });

  it('stays off, and says why, when the feature is not turned on', async () => {
    stub(support({ enabled: false, allowed: false, reason: 'node shells are off; turn them on' }));

    render(<NodeShellButton node="p-mk1" />);

    expect(await screen.findByTitle('node shells are off; turn them on')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Node shell' })).toBeDisabled();
  });

  it('stays off when the cluster refuses the pod', async () => {
    stub(support({ allowed: false, reason: 'you may not create pods in kube-system' }));

    render(<NodeShellButton node="p-mk1" />);

    expect(await screen.findByTitle('you may not create pods in kube-system')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Node shell' })).toBeDisabled();
  });

  it('stays off and explains when the question itself fails', async () => {
    stub({ message: 'nope' }, false);

    render(<NodeShellButton node="p-mk1" />);

    expect(screen.getByRole('button', { name: 'Node shell' })).toBeDisabled();
    expect(await screen.findByTitle('nope')).toBeInTheDocument();
    expect(terminalsNow()).toHaveLength(0);
  });

  it('explains a support failure that is not an Error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('failed'));

    render(<NodeShellButton node="p-mk1" />);

    expect(await screen.findByTitle('the node shell support check failed')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Node shell' })).toBeDisabled();
  });

  it('asks again when the setting turns node shells on', async () => {
    stub(support({ enabled: false, allowed: false, reason: 'node shells are off' }));
    render(<NodeShellButton node="p-mk1" />);
    await screen.findByTitle('node shells are off');

    stub(support());
    act(() => {
      useSettingsStore.setState({ nodeShell: true });
    });

    await vi.waitFor(() => {
      expect(screen.getByRole('button', { name: 'Node shell' })).toBeEnabled();
    });
  });

  it('asks again when another node is selected', async () => {
    stub(support());
    const view = render(<NodeShellButton node="p-mk1" />);
    await screen.findByTitle(/Opens a root shell on p-mk1/);

    stub(support({ node: 'p-mk2' }));
    view.rerender(<NodeShellButton node="p-mk2" />);

    expect(await screen.findByTitle(/Opens a root shell on p-mk2/)).toBeInTheDocument();
  });

  it('disables the old clusters answer while the replacement loads', async () => {
    const pending = new Promise<never>(() => undefined);
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve(support()),
      })
      .mockImplementationOnce(() => pending);
    vi.stubGlobal('fetch', fetchMock);
    const view = render(<NodeShellButton node="p-mk1" />);
    await vi.waitFor(() => {
      expect(screen.getByRole('button', { name: 'Node shell' })).toBeEnabled();
    });

    await act(async () => {
      bumpClusterEpoch();
      await Promise.resolve();
    });

    expect(screen.getByRole('button', { name: 'Node shell' })).toBeDisabled();
    expect(screen.getByTitle('Checking whether a node shell can be opened')).toBeInTheDocument();
    await vi.waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2);
    });
    view.unmount();
  });
});

import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import NodeShellButton from '../../src/components/NodeShellButton';
import { useTerminalsStore } from '../../src/store/terminals';
import { useSettingsStore } from '../../src/store/settings';

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

    const sessions = useTerminalsStore.getState().sessions;
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

  it('stays off when the question itself fails', () => {
    stub({ message: 'nope' }, false);

    render(<NodeShellButton node="p-mk1" />);

    expect(screen.getByRole('button', { name: 'Node shell' })).toBeDisabled();
    expect(useTerminalsStore.getState().sessions).toHaveLength(0);
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
});

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import BottomDock, { LOGS_SUB_ID } from '../../src/components/BottomDock';
import type { PodTarget } from '../../src/components/BottomDock';
import { useLogsStore } from '../../src/store/logs';

vi.mock('../../src/components/DebugPrompt', () => ({
  default: ({
    target,
    onAttached,
  }: {
    target: { container: string };
    onAttached: (name: string) => void;
  }) => (
    <div data-testid="debug-prompt">
      no shell in {target.container}
      <button
        type="button"
        onClick={() => {
          onAttached('spinoza-debug-1');
        }}
      >
        Attach debug container
      </button>
    </div>
  ),
}));

vi.mock('../../src/components/TerminalPanel', () => ({
  default: ({
    target,
    onShellMissing,
  }: {
    target: { pod: string; container: string };
    onShellMissing: () => void;
  }) => (
    <div data-testid="terminal-panel">
      {target.pod}/{target.container}
      <button type="button" onClick={onShellMissing}>
        Report missing shell
      </button>
    </div>
  ),
}));

function stubFetch(shell: string) {
  vi.stubGlobal(
    'fetch',
    vi.fn((input: string) => {
      if (input.startsWith('/api/exec/support')) {
        return Promise.resolve({ ok: true, json: () => Promise.resolve({ shell }) });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve([]) });
    }),
  );
}

function pod(overrides: Partial<PodTarget> = {}): PodTarget {
  return {
    namespace: 'flux-system',
    name: 'web',
    containers: ['app'],
    ...overrides,
  };
}

function renderDock(target: PodTarget | null) {
  const subscribeLogs = vi.fn();
  const unsubscribeLogs = vi.fn();
  render(
    <BottomDock pod={target} subscribeLogs={subscribeLogs} unsubscribeLogs={unsubscribeLogs} />,
  );
  return { subscribeLogs, unsubscribeLogs };
}

describe('BottomDock', () => {
  beforeEach(() => {
    useLogsStore.setState({ streams: new Map() });
    stubFetch('unknown');
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders collapsed with no log body', () => {
    renderDock(pod());
    expect(screen.getByRole('button', { name: 'Toggle panel' })).toHaveTextContent('▸');
    expect(screen.queryByText('Waiting for output…')).not.toBeInTheDocument();
  });

  it('opens and closes from the chevron', async () => {
    const user = userEvent.setup();
    renderDock(null);
    const toggle = screen.getByRole('button', { name: 'Toggle panel' });

    await user.click(toggle);

    expect(toggle).toHaveTextContent('▾');
    expect(screen.getByText('Select a pod to stream its logs.')).toBeInTheDocument();

    await user.click(toggle);

    expect(toggle).toHaveTextContent('▸');
    expect(screen.queryByText('Select a pod to stream its logs.')).not.toBeInTheDocument();
  });

  it('subscribes when opened with a pod and unsubscribes on close', async () => {
    const user = userEvent.setup();
    const { subscribeLogs, unsubscribeLogs } = renderDock(pod());

    await user.click(screen.getByRole('button', { name: 'Logs' }));

    expect(subscribeLogs).toHaveBeenCalledWith(LOGS_SUB_ID, {
      namespace: 'flux-system',
      name: 'web',
      container: 'app',
      tailLines: 500,
      follow: true,
    });

    await user.click(screen.getByRole('button', { name: 'Toggle panel' }));

    expect(unsubscribeLogs).toHaveBeenCalledWith(LOGS_SUB_ID);
  });

  it('does not subscribe without a pod', async () => {
    const user = userEvent.setup();
    const { subscribeLogs } = renderDock(null);

    await user.click(screen.getByRole('button', { name: 'Logs' }));

    expect(subscribeLogs).not.toHaveBeenCalled();
  });

  it('does not subscribe when the pod has no containers', async () => {
    const user = userEvent.setup();
    const { subscribeLogs } = renderDock(pod({ containers: [] }));

    await user.click(screen.getByRole('button', { name: 'Logs' }));

    expect(subscribeLogs).not.toHaveBeenCalled();
  });

  it('renders streamed lines and the waiting placeholder', async () => {
    const user = userEvent.setup();
    renderDock(pod());
    await user.click(screen.getByRole('button', { name: 'Logs' }));

    expect(screen.getByText('Waiting for output…')).toBeInTheDocument();

    useLogsStore.getState().startStream(LOGS_SUB_ID);
    useLogsStore.getState().appendLines(LOGS_SUB_ID, ['first line', 'second line']);

    expect(await screen.findByText('first line')).toBeInTheDocument();
    expect(screen.getByText('second line')).toBeInTheDocument();
    expect(screen.queryByText('Waiting for output…')).not.toBeInTheDocument();
  });

  it('shows the stream-ended marker', async () => {
    const user = userEvent.setup();
    renderDock(pod());
    await user.click(screen.getByRole('button', { name: 'Logs' }));

    useLogsStore.getState().startStream(LOGS_SUB_ID);
    useLogsStore.getState().endStream(LOGS_SUB_ID);

    expect(await screen.findByText('stream ended')).toBeInTheDocument();
  });

  it('shows why a log stream failed instead of just ending it', async () => {
    const user = userEvent.setup();
    renderDock(pod());
    await user.click(screen.getByRole('button', { name: 'Logs' }));

    useLogsStore.getState().startStream(LOGS_SUB_ID);
    useLogsStore.getState().failStream(LOGS_SUB_ID, 'pods/log is forbidden');

    expect(await screen.findByText('pods/log is forbidden')).toBeInTheDocument();
    expect(screen.queryByText('stream ended')).not.toBeInTheDocument();
  });

  it('ignores a failure for a stream it never started', async () => {
    const user = userEvent.setup();
    renderDock(pod());
    await user.click(screen.getByRole('button', { name: 'Logs' }));

    useLogsStore.getState().failStream(LOGS_SUB_ID, 'too late');

    expect(screen.queryByText('too late')).not.toBeInTheDocument();
  });

  it('keeps the scrollback when follow is paused', async () => {
    const user = userEvent.setup();
    renderDock(pod());
    await user.click(screen.getByRole('button', { name: 'Logs' }));

    act(() => {
      useLogsStore.getState().startStream(LOGS_SUB_ID);
      useLogsStore.getState().appendLines(LOGS_SUB_ID, ['first line', 'second line']);
    });
    expect(await screen.findByText('first line')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Following' }));

    expect(screen.getByText('first line')).toBeInTheDocument();
    expect(screen.getByText('second line')).toBeInTheDocument();
  });

  it('toggles follow and resubscribes', async () => {
    const user = userEvent.setup();
    const { subscribeLogs } = renderDock(pod());
    await user.click(screen.getByRole('button', { name: 'Logs' }));

    await user.click(screen.getByRole('button', { name: 'Following' }));

    expect(screen.getByRole('button', { name: 'Paused' })).toBeInTheDocument();
    expect(subscribeLogs).toHaveBeenLastCalledWith(
      LOGS_SUB_ID,
      expect.objectContaining({ follow: false }),
    );
  });

  it('offers a container picker for multi-container pods', async () => {
    const user = userEvent.setup();
    const { subscribeLogs } = renderDock(pod({ containers: ['app', 'sidecar'] }));
    await user.click(screen.getByRole('button', { name: 'Logs' }));

    await user.selectOptions(screen.getByLabelText('Container'), 'sidecar');

    expect(subscribeLogs).toHaveBeenLastCalledWith(
      LOGS_SUB_ID,
      expect.objectContaining({ container: 'sidecar' }),
    );
  });

  it('hides the container picker for single-container pods', async () => {
    const user = userEvent.setup();
    renderDock(pod());
    await user.click(screen.getByRole('button', { name: 'Logs' }));

    expect(screen.queryByLabelText('Container')).not.toBeInTheDocument();
  });

  it('keeps the view pinned to the newest line while following', async () => {
    const user = userEvent.setup();
    renderDock(pod());
    await user.click(screen.getByRole('button', { name: 'Logs' }));

    useLogsStore.getState().startStream(LOGS_SUB_ID);
    useLogsStore.getState().appendLines(LOGS_SUB_ID, ['line']);
    const body = await screen.findByText('line');
    const scroller = body.parentElement;

    expect(scroller).not.toBeNull();
    expect(scroller?.scrollTop).toBe(scroller?.scrollHeight);
  });

  it('stops pinning when follow is off', async () => {
    const user = userEvent.setup();
    renderDock(pod());
    await user.click(screen.getByRole('button', { name: 'Logs' }));
    await user.click(screen.getByRole('button', { name: 'Following' }));

    useLogsStore.getState().startStream(LOGS_SUB_ID);
    useLogsStore.getState().appendLines(LOGS_SUB_ID, ['line']);

    expect(await screen.findByText('line')).toBeInTheDocument();
  });
  it('switches to the forwards tab and stops streaming logs', async () => {
    const user = userEvent.setup();
    const { subscribeLogs, unsubscribeLogs } = renderDock(pod());
    await user.click(screen.getByRole('button', { name: 'Logs' }));
    expect(subscribeLogs).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole('button', { name: 'Forwards' }));

    expect(await screen.findByText(/No active forwards/)).toBeInTheDocument();
    expect(unsubscribeLogs).toHaveBeenCalledWith(LOGS_SUB_ID);
    expect(screen.queryByRole('button', { name: 'Following' })).not.toBeInTheDocument();
  });

  it('collapses from the chevron', async () => {
    const user = userEvent.setup();
    renderDock(pod());
    await user.click(screen.getByRole('button', { name: 'Logs' }));
    expect(screen.getByRole('button', { name: 'Following' })).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Toggle panel' }));

    expect(screen.queryByRole('button', { name: 'Following' })).not.toBeInTheDocument();
  });

  it('opens a terminal for the selected container', async () => {
    const user = userEvent.setup();
    const { unsubscribeLogs } = renderDock(pod());
    await user.click(screen.getByRole('button', { name: 'Logs' }));

    await user.click(screen.getByRole('button', { name: 'Terminal' }));

    expect(await screen.findByTestId('terminal-panel')).toHaveTextContent('web/app');
    expect(unsubscribeLogs).toHaveBeenCalledWith(LOGS_SUB_ID);
  });

  it('keeps the terminal shut without a pod', async () => {
    const user = userEvent.setup();
    renderDock(null);
    await user.click(screen.getByRole('button', { name: 'Toggle panel' }));

    expect(screen.getByRole('button', { name: 'Terminal' })).toBeDisabled();
  });

  it('keeps the terminal shut for a pod with no containers', () => {
    renderDock(pod({ containers: [] }));

    expect(screen.getByRole('button', { name: 'Terminal' })).toBeDisabled();
  });

  it('offers a debug container for an image with no shell', async () => {
    const user = userEvent.setup();
    stubFetch('absent');
    renderDock(pod());

    await user.click(screen.getByRole('button', { name: 'Terminal' }));

    expect(await screen.findByTestId('debug-prompt')).toHaveTextContent('no shell in app');
    expect(screen.queryByTestId('terminal-panel')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Terminal' })).toHaveAttribute(
      'title',
      'No shell in this image — a debug container can be attached',
    );
  });

  it('opens a terminal into the debug container once attached', async () => {
    const user = userEvent.setup();
    stubFetch('absent');
    renderDock(pod());
    await user.click(screen.getByRole('button', { name: 'Terminal' }));
    await screen.findByTestId('debug-prompt');

    await user.click(screen.getByRole('button', { name: 'Attach debug container' }));

    expect(await screen.findByTestId('terminal-panel')).toHaveTextContent('web/spinoza-debug-1');
    expect(screen.queryByTestId('debug-prompt')).not.toBeInTheDocument();
  });

  it('offers a debug container when the next container has no shell', async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      'fetch',
      vi.fn((input: string) => {
        let shell = 'unknown';
        if (input.includes('container=sidecar')) {
          shell = 'absent';
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({ shell }) });
      }),
    );
    renderDock(pod({ containers: ['app', 'sidecar'] }));
    await user.click(screen.getByRole('button', { name: 'Terminal' }));
    expect(await screen.findByTestId('terminal-panel')).toHaveTextContent('web/app');

    await user.selectOptions(screen.getByLabelText('Container'), 'sidecar');

    await waitFor(() => {
      expect(screen.queryByTestId('terminal-panel')).not.toBeInTheDocument();
    });
    expect(screen.getByTestId('debug-prompt')).toHaveTextContent('no shell in sidecar');
  });

  it('shows the container picker on the terminal tab', async () => {
    const user = userEvent.setup();
    renderDock(pod({ containers: ['app', 'sidecar'] }));

    await user.click(screen.getByRole('button', { name: 'Terminal' }));

    expect(screen.getByLabelText('Container')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Following' })).not.toBeInTheDocument();
  });

  it('survives a failed support lookup', async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.reject(new Error('offline'))),
    );
    renderDock(pod());

    await user.click(screen.getByRole('button', { name: 'Terminal' }));

    expect(await screen.findByTestId('terminal-panel')).toBeInTheDocument();
  });

  it('offers a debug container when the session reports no shell', async () => {
    const user = userEvent.setup();
    renderDock(pod());
    await user.click(screen.getByRole('button', { name: 'Terminal' }));

    await user.click(screen.getByRole('button', { name: 'Report missing shell' }));

    expect(screen.queryByTestId('terminal-panel')).not.toBeInTheDocument();
    expect(screen.getByTestId('debug-prompt')).toBeInTheDocument();
  });
});

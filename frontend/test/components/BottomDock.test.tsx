import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import BottomDock, { LOGS_SUB_ID } from '../../src/components/BottomDock';
import type { PodTarget } from '../../src/components/BottomDock';
import { useLogsStore } from '../../src/store/logs';

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
  });

  it('renders collapsed with no log body', () => {
    renderDock(pod());
    expect(screen.getByRole('button', { name: /Logs/ })).toHaveTextContent('▸');
    expect(screen.queryByRole('button', { name: 'Terminal' })).not.toBeInTheDocument();
  });

  it('opens and closes on toggle', async () => {
    const user = userEvent.setup();
    renderDock(null);
    const toggle = screen.getByRole('button', { name: /Logs/ });

    await user.click(toggle);

    expect(toggle).toHaveTextContent('▾');
    expect(screen.getByText('Select a pod to stream its logs.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Terminal' })).toBeDisabled();

    await user.click(toggle);

    expect(toggle).toHaveTextContent('▸');
    expect(screen.queryByText('Select a pod to stream its logs.')).not.toBeInTheDocument();
  });

  it('subscribes when opened with a pod and unsubscribes on close', async () => {
    const user = userEvent.setup();
    const { subscribeLogs, unsubscribeLogs } = renderDock(pod());

    await user.click(screen.getByRole('button', { name: /Logs/ }));

    expect(subscribeLogs).toHaveBeenCalledWith(LOGS_SUB_ID, {
      namespace: 'flux-system',
      name: 'web',
      container: 'app',
      tailLines: 500,
      follow: true,
    });

    await user.click(screen.getByRole('button', { name: /Logs/ }));

    expect(unsubscribeLogs).toHaveBeenCalledWith(LOGS_SUB_ID);
  });

  it('does not subscribe without a pod', async () => {
    const user = userEvent.setup();
    const { subscribeLogs } = renderDock(null);

    await user.click(screen.getByRole('button', { name: /Logs/ }));

    expect(subscribeLogs).not.toHaveBeenCalled();
  });

  it('does not subscribe when the pod has no containers', async () => {
    const user = userEvent.setup();
    const { subscribeLogs } = renderDock(pod({ containers: [] }));

    await user.click(screen.getByRole('button', { name: /Logs/ }));

    expect(subscribeLogs).not.toHaveBeenCalled();
  });

  it('renders streamed lines and the waiting placeholder', async () => {
    const user = userEvent.setup();
    renderDock(pod());
    await user.click(screen.getByRole('button', { name: /Logs/ }));

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
    await user.click(screen.getByRole('button', { name: /Logs/ }));

    useLogsStore.getState().startStream(LOGS_SUB_ID);
    useLogsStore.getState().endStream(LOGS_SUB_ID);

    expect(await screen.findByText('stream ended')).toBeInTheDocument();
  });

  it('toggles follow and resubscribes', async () => {
    const user = userEvent.setup();
    const { subscribeLogs } = renderDock(pod());
    await user.click(screen.getByRole('button', { name: /Logs/ }));

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
    await user.click(screen.getByRole('button', { name: /Logs/ }));

    await user.selectOptions(screen.getByLabelText('Container'), 'sidecar');

    expect(subscribeLogs).toHaveBeenLastCalledWith(
      LOGS_SUB_ID,
      expect.objectContaining({ container: 'sidecar' }),
    );
  });

  it('hides the container picker for single-container pods', async () => {
    const user = userEvent.setup();
    renderDock(pod());
    await user.click(screen.getByRole('button', { name: /Logs/ }));

    expect(screen.queryByLabelText('Container')).not.toBeInTheDocument();
  });

  it('keeps the view pinned to the newest line while following', async () => {
    const user = userEvent.setup();
    renderDock(pod());
    await user.click(screen.getByRole('button', { name: /Logs/ }));

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
    await user.click(screen.getByRole('button', { name: /Logs/ }));
    await user.click(screen.getByRole('button', { name: 'Following' }));

    useLogsStore.getState().startStream(LOGS_SUB_ID);
    useLogsStore.getState().appendLines(LOGS_SUB_ID, ['line']);

    expect(await screen.findByText('line')).toBeInTheDocument();
  });
});

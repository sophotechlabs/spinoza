import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import InspectLogs, { INSPECT_LOGS_SUB_ID } from '../../src/components/InspectLogs';
import { scrollToBottom } from '../../src/lib/scroll';
import { useLogsStore } from '../../src/store/logs';

function renderLogs(props: { namespace?: string; pod?: string; containers?: string[] } = {}) {
  const subscribeLogs = vi.fn();
  const unsubscribeLogs = vi.fn();
  const view = render(
    <InspectLogs
      namespace={props.namespace ?? 'flux-system'}
      pod={props.pod ?? 'web'}
      containers={props.containers ?? ['app', 'sidecar']}
      subscribeLogs={subscribeLogs}
      unsubscribeLogs={unsubscribeLogs}
    />,
  );
  return { subscribeLogs, unsubscribeLogs, view };
}

describe('InspectLogs', () => {
  beforeEach(() => {
    useLogsStore.setState({ streams: new Map() });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('waits for output before any line arrives', () => {
    renderLogs();

    expect(screen.getByText('Waiting for output…')).toBeInTheDocument();
  });

  it('resets the container when the pod changes', () => {
    const { subscribeLogs, view } = renderLogs();

    view.rerender(
      <InspectLogs
        namespace="flux-system"
        pod="api"
        containers={['server']}
        subscribeLogs={subscribeLogs}
        unsubscribeLogs={vi.fn()}
      />,
    );

    expect(subscribeLogs).toHaveBeenLastCalledWith(
      INSPECT_LOGS_SUB_ID,
      expect.objectContaining({ name: 'api', container: 'server' }),
    );
  });

  it('subscribes with no container when the pod reports none', () => {
    const { subscribeLogs } = renderLogs({ containers: [] });

    expect(subscribeLogs).not.toHaveBeenCalled();
    expect(screen.queryByLabelText('Log container')).not.toBeInTheDocument();
  });

  it('hides the picker for a single-container pod', () => {
    renderLogs({ containers: ['only'] });

    expect(screen.queryByLabelText('Log container')).not.toBeInTheDocument();
  });

  it('scrolls to the newest line while following', () => {
    renderLogs();
    const body = screen.getByText('Waiting for output…').parentElement as HTMLDivElement;
    vi.spyOn(body, 'scrollHeight', 'get').mockReturnValue(900);

    act(() => {
      useLogsStore.getState().startStream(INSPECT_LOGS_SUB_ID);
      useLogsStore.getState().appendLines(INSPECT_LOGS_SUB_ID, ['one']);
    });

    expect(body.scrollTop).toBe(900);
  });

  it('leaves the scroll position alone once paused', async () => {
    const user = userEvent.setup();
    renderLogs();
    const body = screen.getByText('Waiting for output…').parentElement as HTMLDivElement;
    vi.spyOn(body, 'scrollHeight', 'get').mockReturnValue(900);
    await user.click(screen.getByRole('button', { name: 'Following' }));

    act(() => {
      useLogsStore.getState().startStream(INSPECT_LOGS_SUB_ID);
      useLogsStore.getState().appendLines(INSPECT_LOGS_SUB_ID, ['one']);
    });

    expect(body.scrollTop).toBe(0);
  });

  it('ignores a missing scroll container', () => {
    expect(() => {
      scrollToBottom(null);
    }).not.toThrow();
  });
});

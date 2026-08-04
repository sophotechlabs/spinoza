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

  it('switches container from the picker', async () => {
    const user = userEvent.setup();
    const { subscribeLogs } = renderLogs();

    await user.selectOptions(screen.getByLabelText('Log container'), 'sidecar');

    expect(subscribeLogs).toHaveBeenLastCalledWith(
      INSPECT_LOGS_SUB_ID,
      expect.objectContaining({ container: 'sidecar' }),
    );
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

describe('InspectLogs stream state', () => {
  it('shows no error banner before a stream exists', () => {
    useLogsStore.setState({ streams: new Map() });
    renderLogs();

    expect(screen.queryByText('stream ended')).not.toBeInTheDocument();
  });

  it('shows why the stream failed', () => {
    useLogsStore.setState({ streams: new Map() });
    renderLogs();

    act(() => {
      useLogsStore.getState().startStream(INSPECT_LOGS_SUB_ID);
      useLogsStore.getState().failStream(INSPECT_LOGS_SUB_ID, 'pods/log is forbidden');
    });

    expect(screen.getByText('pods/log is forbidden')).toBeInTheDocument();
  });

  it('marks a stream that ended', () => {
    useLogsStore.setState({ streams: new Map() });
    renderLogs();

    act(() => {
      useLogsStore.getState().startStream(INSPECT_LOGS_SUB_ID);
      useLogsStore.getState().endStream(INSPECT_LOGS_SUB_ID);
    });

    expect(screen.getByText('stream ended')).toBeInTheDocument();
  });
});

describe('reading a structured log line', () => {
  const jsonLine =
    '{"level":"info","ts":"2026-08-04T11:56:53.059Z","caller":"http/server.go:273","msg":"Starting HTTP Server.","addr":":9898"}';

  function pushLine() {
    act(() => {
      useLogsStore.getState().startStream(INSPECT_LOGS_SUB_ID);
      useLogsStore.getState().appendLines(INSPECT_LOGS_SUB_ID, [jsonLine]);
    });
  }

  beforeEach(() => {
    useLogsStore.setState({ streams: new Map() });
  });

  it('formats it for reading by default', () => {
    renderLogs();
    pushLine();

    expect(screen.getByRole('button', { name: 'Pretty' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByText('Starting HTTP Server.')).toBeInTheDocument();
    expect(screen.queryByText(jsonLine)).not.toBeInTheDocument();
  });

  it('hands back the untouched line when asked for raw', async () => {
    const user = userEvent.setup();
    renderLogs();
    pushLine();

    await user.click(screen.getByRole('button', { name: 'Pretty' }));

    expect(screen.getByRole('button', { name: 'Raw' })).toHaveAttribute('aria-pressed', 'false');
    expect(screen.getByText(jsonLine)).toBeInTheDocument();
    expect(screen.queryByText('Starting HTTP Server.')).not.toBeInTheDocument();
  });
});

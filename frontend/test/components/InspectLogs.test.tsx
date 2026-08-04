import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import InspectLogs from '../../src/components/InspectLogs';
import { scrollToBottom } from '../../src/lib/scroll';
import { useLogsStore } from '../../src/store/logs';

type SubscribeLogs = ReturnType<typeof vi.fn<(subId: string, request: unknown) => void>>;

function renderLogs(props: { namespace?: string; pod?: string; containers?: string[] } = {}) {
  const subscribeLogs = vi.fn<(subId: string, request: unknown) => void>();
  const unsubscribeLogs = vi.fn<(subId: string) => void>();
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

function liveSubId(subscribeLogs: SubscribeLogs): string {
  const calls = subscribeLogs.mock.calls;
  return calls[calls.length - 1][0];
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
      liveSubId(subscribeLogs),
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
      liveSubId(subscribeLogs),
      expect.objectContaining({ container: 'sidecar' }),
    );
  });

  it('reads the new stream only, so the previous container cannot bleed in', async () => {
    const user = userEvent.setup();
    const { subscribeLogs } = renderLogs();
    const first = liveSubId(subscribeLogs);
    act(() => {
      useLogsStore.getState().startStream(first);
      useLogsStore.getState().appendLines(first, ['from-app']);
    });
    expect(screen.getByText('from-app')).toBeInTheDocument();

    await user.selectOptions(screen.getByLabelText('Log container'), 'sidecar');
    const second = liveSubId(subscribeLogs);

    expect(second).not.toBe(first);
    act(() => {
      useLogsStore.getState().appendLines(first, ['late-from-app']);
    });
    expect(screen.queryByText('late-from-app')).not.toBeInTheDocument();
    expect(screen.getByText('Waiting for output…')).toBeInTheDocument();
  });

  it('scrolls to the newest line while following', () => {
    const { subscribeLogs } = renderLogs();
    const subId = liveSubId(subscribeLogs);
    const body = screen.getByText('Waiting for output…').parentElement as HTMLDivElement;
    vi.spyOn(body, 'scrollHeight', 'get').mockReturnValue(900);

    act(() => {
      useLogsStore.getState().startStream(subId);
      useLogsStore.getState().appendLines(subId, ['one']);
    });

    expect(body.scrollTop).toBe(900);
  });

  it('leaves the scroll position alone once paused', async () => {
    const user = userEvent.setup();
    const { subscribeLogs } = renderLogs();
    const subId = liveSubId(subscribeLogs);
    const body = screen.getByText('Waiting for output…').parentElement as HTMLDivElement;
    vi.spyOn(body, 'scrollHeight', 'get').mockReturnValue(900);
    await user.click(screen.getByRole('button', { name: 'Following' }));

    act(() => {
      useLogsStore.getState().startStream(subId);
      useLogsStore.getState().appendLines(subId, ['one']);
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
    const { subscribeLogs } = renderLogs();
    const subId = liveSubId(subscribeLogs);

    act(() => {
      useLogsStore.getState().startStream(subId);
      useLogsStore.getState().failStream(subId, 'pods/log is forbidden');
    });

    expect(screen.getByText('pods/log is forbidden')).toBeInTheDocument();
  });

  it('marks a stream that ended', () => {
    useLogsStore.setState({ streams: new Map() });
    const { subscribeLogs } = renderLogs();
    const subId = liveSubId(subscribeLogs);

    act(() => {
      useLogsStore.getState().startStream(subId);
      useLogsStore.getState().endStream(subId);
    });

    expect(screen.getByText('stream ended')).toBeInTheDocument();
  });
});

describe('reading a structured log line', () => {
  const jsonLine =
    '{"level":"info","ts":"2026-08-04T11:56:53.059Z","caller":"http/server.go:273","msg":"Starting HTTP Server.","addr":":9898"}';

  function pushLine(subscribeLogs: SubscribeLogs) {
    const subId = liveSubId(subscribeLogs);
    act(() => {
      useLogsStore.getState().startStream(subId);
      useLogsStore.getState().appendLines(subId, [jsonLine]);
    });
  }

  beforeEach(() => {
    useLogsStore.setState({ streams: new Map() });
  });

  it('formats it for reading by default', () => {
    const { subscribeLogs } = renderLogs();
    pushLine(subscribeLogs);

    expect(screen.getByRole('button', { name: 'Pretty' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByText('Starting HTTP Server.')).toBeInTheDocument();
    expect(screen.queryByText(jsonLine)).not.toBeInTheDocument();
  });

  it('hands back the untouched line when asked for raw', async () => {
    const user = userEvent.setup();
    const { subscribeLogs } = renderLogs();
    pushLine(subscribeLogs);

    await user.click(screen.getByRole('button', { name: 'Pretty' }));

    expect(screen.getByRole('button', { name: 'Raw' })).toHaveAttribute('aria-pressed', 'false');
    expect(screen.getByText(jsonLine)).toBeInTheDocument();
    expect(screen.queryByText('Starting HTTP Server.')).not.toBeInTheDocument();
  });
});

describe('a very long log buffer', () => {
  beforeEach(() => {
    useLogsStore.setState({ streams: new Map() });
  });

  it('puts far fewer lines in the dom than the buffer holds', () => {
    const { subscribeLogs } = renderLogs();
    const subId = liveSubId(subscribeLogs);
    act(() => {
      useLogsStore.getState().startStream(subId);
      useLogsStore.getState().appendLines(
        subId,
        Array.from({ length: 5000 }, (_unused, index) => `line ${String(index)}`),
      );
    });

    expect(screen.getByText('line 0')).toBeInTheDocument();
    expect(screen.queryByText('line 4999')).not.toBeInTheDocument();
    expect(document.querySelectorAll('[data-index]').length).toBeLessThan(200);
  });

  it('tags every rendered line with its place in the buffer', () => {
    const { subscribeLogs } = renderLogs();
    const subId = liveSubId(subscribeLogs);
    act(() => {
      useLogsStore.getState().startStream(subId);
      useLogsStore.getState().appendLines(subId, ['alpha', 'bravo']);
    });

    const rendered = [...document.querySelectorAll<HTMLElement>('[data-index]')];
    expect(rendered.map((node) => node.dataset.index)).toEqual(['0', '1']);
    expect(rendered[0].className).toContain('absolute');
  });

  it('keeps the window small while the buffer keeps growing', () => {
    const { subscribeLogs } = renderLogs();
    const subId = liveSubId(subscribeLogs);
    act(() => {
      useLogsStore.getState().startStream(subId);
      useLogsStore.getState().appendLines(subId, ['only line']);
    });
    const small = document.querySelectorAll('[data-index]').length;

    act(() => {
      useLogsStore.getState().appendLines(
        subId,
        Array.from({ length: 4000 }, (_unused, index) => `more ${String(index)}`),
      );
    });

    expect(document.querySelectorAll('[data-index]').length).toBeLessThan(small + 200);
  });
});

describe('a logs panel behind another tab', () => {
  beforeEach(() => {
    useLogsStore.setState({ streams: new Map() });
  });

  it('never opens a stream while it is hidden', () => {
    const subscribeLogs = vi.fn<(subId: string, request: unknown) => void>();
    render(
      <InspectLogs
        namespace="flux-system"
        pod="web"
        containers={['app']}
        active={false}
        subscribeLogs={subscribeLogs}
        unsubscribeLogs={vi.fn()}
      />,
    );

    expect(subscribeLogs).not.toHaveBeenCalled();
  });

  it('drops the stream when its tab goes away and reopens it on return', () => {
    const subscribeLogs = vi.fn<(subId: string, request: unknown) => void>();
    const unsubscribeLogs = vi.fn<(subId: string) => void>();
    const props = {
      namespace: 'flux-system',
      pod: 'web',
      containers: ['app'],
      subscribeLogs,
      unsubscribeLogs,
    };
    const view = render(<InspectLogs {...props} active />);
    const first = liveSubId(subscribeLogs);

    view.rerender(<InspectLogs {...props} active={false} />);
    expect(unsubscribeLogs).toHaveBeenCalledWith(first);

    view.rerender(<InspectLogs {...props} active />);
    expect(liveSubId(subscribeLogs)).not.toBe(first);
  });
});

describe('working through a log buffer', () => {
  beforeEach(() => {
    useLogsStore.setState({ streams: new Map() });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  function seedLines(subscribeLogs: SubscribeLogs, lines: string[]): string {
    const subId = liveSubId(subscribeLogs);
    act(() => {
      useLogsStore.getState().startStream(subId);
      useLogsStore.getState().appendLines(subId, lines);
    });
    return subId;
  }

  it('keeps only the lines that match the filter', async () => {
    const user = userEvent.setup();
    const { subscribeLogs } = renderLogs();
    seedLines(subscribeLogs, ['alpha one', 'bravo two', 'alpha three']);

    await user.type(screen.getByLabelText('Filter log lines'), 'alpha');

    expect(screen.getByText('alpha one')).toBeInTheDocument();
    expect(screen.queryByText('bravo two')).not.toBeInTheDocument();
    expect(screen.getByText('2 of 3')).toBeInTheDocument();
  });

  it('ignores case while filtering', async () => {
    const user = userEvent.setup();
    const { subscribeLogs } = renderLogs();
    seedLines(subscribeLogs, ['Alpha one']);

    await user.type(screen.getByLabelText('Filter log lines'), 'ALPHA');

    expect(screen.getByText('Alpha one')).toBeInTheDocument();
  });

  it('says when the filter matches nothing', async () => {
    const user = userEvent.setup();
    const { subscribeLogs } = renderLogs();
    seedLines(subscribeLogs, ['alpha one']);

    await user.type(screen.getByLabelText('Filter log lines'), 'zzz');

    expect(screen.getByText('No line matches that filter.')).toBeInTheDocument();
  });

  it('empties the buffer on Clear', async () => {
    const user = userEvent.setup();
    const { subscribeLogs } = renderLogs();
    seedLines(subscribeLogs, ['alpha one']);

    await user.click(screen.getByRole('button', { name: 'Clear' }));

    expect(screen.getByText('Waiting for output…')).toBeInTheDocument();
  });

  it('downloads what is on screen, named after the pod', async () => {
    const user = userEvent.setup();
    const createObjectURL = vi.fn().mockReturnValue('blob:log');
    const revokeObjectURL = vi.fn();
    vi.stubGlobal('URL', { ...URL, createObjectURL, revokeObjectURL });
    const clicks: string[] = [];
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function record(
      this: HTMLAnchorElement,
    ) {
      clicks.push(this.download);
    });
    const { subscribeLogs } = renderLogs();
    seedLines(subscribeLogs, ['alpha one', 'bravo two']);

    await user.click(screen.getByRole('button', { name: 'Download' }));

    expect(createObjectURL).toHaveBeenCalledTimes(1);
    expect(clicks).toEqual(['flux-system-web-app.log']);
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:log');
    click.mockRestore();
  });

  it('names the file after the pod alone when it has no containers', async () => {
    const user = userEvent.setup();
    const clicks: string[] = [];
    vi.stubGlobal('URL', {
      ...URL,
      createObjectURL: () => 'blob:log',
      revokeObjectURL: vi.fn(),
    });
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function record(
      this: HTMLAnchorElement,
    ) {
      clicks.push(this.download);
    });
    renderLogs({ containers: [] });

    await user.click(screen.getByRole('button', { name: 'Download' }));

    expect(clicks).toEqual(['flux-system-web.log']);
    click.mockRestore();
  });

  it('downloads only the filtered lines', async () => {
    const user = userEvent.setup();
    let saved = '';
    vi.stubGlobal('URL', {
      ...URL,
      createObjectURL: (blob: Blob) => {
        saved = String(blob.size);
        return 'blob:log';
      },
      revokeObjectURL: vi.fn(),
    });
    const click = vi
      .spyOn(HTMLAnchorElement.prototype, 'click')
      .mockImplementation(() => undefined);
    const { subscribeLogs } = renderLogs();
    seedLines(subscribeLogs, ['alpha one', 'bravo two']);

    await user.type(screen.getByLabelText('Filter log lines'), 'alpha');
    await user.click(screen.getByRole('button', { name: 'Download' }));

    expect(saved).toBe(String('alpha one'.length));
    click.mockRestore();
  });

  it('drops the timestamp from a line when asked', async () => {
    const user = userEvent.setup();
    const { subscribeLogs } = renderLogs();
    seedLines(subscribeLogs, ['{"level":"info","ts":"2026-08-04T10:11:12Z","msg":"up"}']);
    expect(screen.getByText('10:11:12', { exact: false })).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Timestamps' }));

    expect(screen.queryByText('10:11:12', { exact: false })).not.toBeInTheDocument();
    expect(screen.getByText('up')).toBeInTheDocument();
  });

  it('stops wrapping long lines when asked', async () => {
    const user = userEvent.setup();
    const { subscribeLogs } = renderLogs();
    seedLines(subscribeLogs, ['a very long line']);
    const line = document.querySelector('[data-index]');
    expect(line?.className).toContain('whitespace-pre-wrap');

    await user.click(screen.getByRole('button', { name: 'Wrap' }));

    expect(document.querySelector('[data-index]')?.className).toContain('whitespace-pre');
  });

  it('offers a jump to the bottom only once follow is off', async () => {
    const user = userEvent.setup();
    const { subscribeLogs } = renderLogs();
    seedLines(subscribeLogs, ['alpha one']);
    expect(screen.queryByRole('button', { name: 'Jump to bottom' })).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Following' }));
    await user.click(screen.getByRole('button', { name: 'Jump to bottom' }));

    expect(screen.getByText('alpha one')).toBeInTheDocument();
  });
});

describe('copying the log buffer', () => {
  beforeEach(() => {
    useLogsStore.setState({ streams: new Map() });
  });

  it('copies what is on screen', async () => {
    const user = userEvent.setup();
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
    const { subscribeLogs } = renderLogs();
    const subId = liveSubId(subscribeLogs);
    act(() => {
      useLogsStore.getState().startStream(subId);
      useLogsStore.getState().appendLines(subId, ['alpha one', 'bravo two']);
    });

    await user.click(screen.getByRole('button', { name: 'Copy' }));

    expect(writeText).toHaveBeenCalledWith('alpha one\nbravo two');
  });
});

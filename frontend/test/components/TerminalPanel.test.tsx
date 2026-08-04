import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import TerminalPanel from '../../src/components/TerminalPanel';
import type { ExecHandlers, ExecSession } from '../../src/lib/exec';
import type { TerminalHandle, TerminalOptions } from '../../src/lib/terminal';
import { terminalTheme } from '../../src/lib/themeColors';
import { BUILT_IN_THEMES, themeById } from '../../src/lib/theme';
import { useThemeStore } from '../../src/store/theme';
import { useSettingsStore } from '../../src/store/settings';

const CONNECT_FAILED = 'could not reach the exec endpoint';

const openExec = vi.fn<(target: unknown, handlers: ExecHandlers) => ExecSession>();
const createTerminal = vi.fn<(node: HTMLElement, options?: TerminalOptions) => TerminalHandle>();

vi.mock('../../src/lib/exec', () => ({
  openExec: (target: unknown, handlers: ExecHandlers) => openExec(target, handlers),
}));

vi.mock('../../src/lib/terminal', () => ({
  createTerminal: (node: HTMLElement, options?: TerminalOptions) => createTerminal(node, options),
}));

interface Harness {
  term: TerminalHandle & { written: string[] };
  session: ExecSession;
  handlers: () => ExecHandlers;
  typed: () => (data: string) => void;
}

function harness(cols = 120, rows = 40): Harness {
  const written: string[] = [];
  let onData: ((data: string) => void) | null = null;

  const term = {
    written,
    write: vi.fn((text: string) => {
      written.push(text);
    }),
    onData: vi.fn((handler: (data: string) => void) => {
      onData = handler;
    }),
    setTheme: vi.fn(),
    setScreenReader: vi.fn(),
    fit: vi.fn(() => ({ cols, rows })),
    focus: vi.fn(),
    dispose: vi.fn(),
  };
  const session = { send: vi.fn(), resize: vi.fn(), close: vi.fn() };

  createTerminal.mockReturnValue(term);
  openExec.mockReturnValue(session);

  return {
    term,
    session,
    handlers: () => openExec.mock.calls[0][1],
    typed: () => onData as unknown as (data: string) => void,
  };
}

function renderPanel(onShellMissing = vi.fn()) {
  const view = render(
    <TerminalPanel
      target={{ namespace: 'monitoring', pod: 'loki-0', container: 'loki' }}
      onShellMissing={onShellMissing}
    />,
  );
  return { view, onShellMissing };
}

describe('TerminalPanel', () => {
  beforeEach(() => {
    openExec.mockReset();
    createTerminal.mockReset();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('opens a session for the target and fits the terminal', () => {
    const { term, session } = harness();
    renderPanel();

    expect(createTerminal).toHaveBeenCalledWith(screen.getByTestId('terminal-host'), {
      screenReader: false,
    });
    expect(openExec.mock.calls[0][0]).toEqual({
      namespace: 'monitoring',
      pod: 'loki-0',
      container: 'loki',
    });
    expect(term.fit).toHaveBeenCalled();
    expect(session.resize).toHaveBeenCalledWith(120, 40);
    expect(term.focus).toHaveBeenCalled();
  });

  it('writes stream output into the terminal', () => {
    const bench = harness();
    renderPanel();

    bench.handlers().onOutput('/ # ');

    expect(bench.term.written).toContain('/ # ');
  });

  it('sends keystrokes to the session', () => {
    const bench = harness();
    renderPanel();

    bench.typed()('ls\n');

    expect(bench.session.send).toHaveBeenCalledWith('ls\n');
  });

  it('shows a clean end without raising the missing-shell flag', () => {
    const bench = harness();
    const { onShellMissing } = renderPanel();

    act(() => {
      bench.handlers().onEnd({ message: '', failed: false });
    });

    expect(bench.term.written.join('')).toContain('session ended');
    expect(onShellMissing).not.toHaveBeenCalled();
  });

  it('surfaces a missing shell and reports it upward', () => {
    const bench = harness();
    const { onShellMissing } = renderPanel();

    act(() => {
      bench
        .handlers()
        .onEnd({ message: 'no shell: loki in monitoring/loki-0 has no /bin/sh', failed: true });
    });

    expect(onShellMissing).toHaveBeenCalled();
    expect(screen.getByText(/has no \/bin\/sh/)).toBeInTheDocument();
  });

  it('does not flag other failures as a missing shell', () => {
    const bench = harness();
    const { onShellMissing } = renderPanel();

    act(() => {
      bench.handlers().onEnd({ message: 'pods "loki-0" is forbidden', failed: true });
    });

    expect(onShellMissing).not.toHaveBeenCalled();
    expect(screen.getByText('pods "loki-0" is forbidden')).toBeInTheDocument();
  });

  it('offers no retry after a clean exit', () => {
    const bench = harness();
    renderPanel();

    act(() => {
      bench.handlers().onEnd({ message: '', failed: false });
    });

    expect(screen.queryByRole('button', { name: 'Retry' })).not.toBeInTheDocument();
  });

  it('reconnects from the retry button after a failed connect', async () => {
    const user = userEvent.setup();
    const bench = harness();
    renderPanel();

    act(() => {
      bench.handlers().onEnd({ message: CONNECT_FAILED, failed: true });
    });
    expect(screen.getByText(CONNECT_FAILED)).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Retry' }));

    expect(openExec).toHaveBeenCalledTimes(2);
    expect(screen.queryByText(CONNECT_FAILED)).not.toBeInTheDocument();
  });

  it('closes the session and disposes the terminal on unmount', () => {
    const bench = harness();
    const { view } = renderPanel();

    view.unmount();

    expect(bench.session.close).toHaveBeenCalled();
    expect(bench.term.dispose).toHaveBeenCalled();
  });
});

describe('a re-render that only changes the callback', () => {
  beforeEach(() => {
    openExec.mockReset();
    createTerminal.mockReset();
  });

  it('keeps the shell open instead of reconnecting', () => {
    const stubs = harness();
    const { view } = renderPanel();

    view.rerender(
      <TerminalPanel
        target={{ namespace: 'monitoring', pod: 'loki-0', container: 'loki' }}
        onShellMissing={vi.fn()}
      />,
    );

    expect(stubs.session.close).not.toHaveBeenCalled();
    expect(openExec).toHaveBeenCalledTimes(1);
  });

  it('still reports a missing shell through the newest callback', () => {
    const stubs = harness();
    const { view } = renderPanel();
    const later = vi.fn();

    view.rerender(
      <TerminalPanel
        target={{ namespace: 'monitoring', pod: 'loki-0', container: 'loki' }}
        onShellMissing={later}
      />,
    );
    act(() => {
      stubs
        .handlers()
        .onEnd({ message: 'exec: "/bin/sh": executable file not found', failed: true });
    });

    expect(later).toHaveBeenCalled();
  });

  it('still reconnects when the container changes', () => {
    const stubs = harness();
    const { view } = renderPanel();

    view.rerender(
      <TerminalPanel
        target={{ namespace: 'monitoring', pod: 'loki-0', container: 'sidecar' }}
        onShellMissing={vi.fn()}
      />,
    );

    expect(stubs.session.close).toHaveBeenCalled();
    expect(openExec).toHaveBeenCalledTimes(2);
  });
});

describe('a live shell when the theme changes', () => {
  beforeEach(() => {
    openExec.mockReset();
    createTerminal.mockReset();
  });

  afterEach(() => {
    act(() => {
      useThemeStore.getState().setPreference('dark');
    });
  });

  it('is recoloured without dropping the session', () => {
    const stubs = harness();
    renderPanel();

    expect(stubs.term.setTheme).toHaveBeenCalledWith(
      terminalTheme(themeById(BUILT_IN_THEMES, 'dark')),
    );

    act(() => {
      useThemeStore.getState().setPreference('light');
    });

    expect(stubs.term.setTheme).toHaveBeenCalledWith(
      terminalTheme(themeById(BUILT_IN_THEMES, 'light')),
    );
    expect(stubs.session.close).not.toHaveBeenCalled();
    expect(openExec).toHaveBeenCalledTimes(1);
    expect(createTerminal).toHaveBeenCalledTimes(1);
  });
});

describe('the terminal and a screen reader', () => {
  beforeEach(() => {
    openExec.mockReset();
    createTerminal.mockReset();
  });

  afterEach(() => {
    act(() => {
      useSettingsStore.getState().setScreenReader(false);
    });
  });

  it('opens with screen reader mode off by default', () => {
    harness();
    renderPanel();

    expect(createTerminal.mock.calls[0][1]).toEqual({ screenReader: false });
  });

  it('opens with it on when the setting says so', () => {
    harness();
    act(() => {
      useSettingsStore.getState().setScreenReader(true);
    });

    renderPanel();

    expect(createTerminal.mock.calls[0][1]).toEqual({ screenReader: true });
  });

  it('turns it on for a live session when the setting changes', () => {
    const bench = harness();
    renderPanel();

    act(() => {
      useSettingsStore.getState().setScreenReader(true);
    });

    expect(bench.term.setScreenReader).toHaveBeenCalledWith(true);
  });
});

describe('the session-end notice', () => {
  beforeEach(() => {
    openExec.mockReset();
    createTerminal.mockReset();
  });

  it('uses the themed sixteen-colour palette, not the 256-colour one', () => {
    const bench = harness();
    renderPanel();

    act(() => {
      bench.handlers().onEnd({ message: '', failed: false });
    });

    const written = bench.term.written.join('');
    expect(written).toContain('\x1b[90msession ended');
    expect(written).not.toContain('38;5;');
  });

  it('paints a failure in the themed yellow', () => {
    const bench = harness();
    renderPanel();

    act(() => {
      bench.handlers().onEnd({ message: 'exec failed', failed: true });
    });

    const written = bench.term.written.join('');
    expect(written).toContain('\x1b[33mexec failed');
    expect(written).not.toContain('38;5;');
  });
});

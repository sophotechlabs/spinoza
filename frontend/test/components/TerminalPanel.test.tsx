import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import TerminalPanel from '../../src/components/TerminalPanel';
import type { ExecHandlers, ExecSession } from '../../src/lib/exec';
import type { TerminalHandle } from '../../src/lib/terminal';

const openExec = vi.fn<(target: unknown, handlers: ExecHandlers) => ExecSession>();
const createTerminal = vi.fn<(node: HTMLElement) => TerminalHandle>();

vi.mock('../../src/lib/exec', () => ({
  openExec: (target: unknown, handlers: ExecHandlers) => openExec(target, handlers),
}));

vi.mock('../../src/lib/terminal', () => ({
  createTerminal: (node: HTMLElement) => createTerminal(node),
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

    expect(createTerminal).toHaveBeenCalledWith(screen.getByTestId('terminal-host'));
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
      bench.handlers().onEnd('');
    });

    expect(bench.term.written.join('')).toContain('session ended');
    expect(onShellMissing).not.toHaveBeenCalled();
  });

  it('surfaces a missing shell and reports it upward', () => {
    const bench = harness();
    const { onShellMissing } = renderPanel();

    act(() => {
      bench.handlers().onEnd('no shell: loki in monitoring/loki-0 has no /bin/sh');
    });

    expect(onShellMissing).toHaveBeenCalled();
    expect(screen.getByText(/has no \/bin\/sh/)).toBeInTheDocument();
  });

  it('does not flag other failures as a missing shell', () => {
    const bench = harness();
    const { onShellMissing } = renderPanel();

    act(() => {
      bench.handlers().onEnd('pods "loki-0" is forbidden');
    });

    expect(onShellMissing).not.toHaveBeenCalled();
    expect(screen.getByText('pods "loki-0" is forbidden')).toBeInTheDocument();
  });

  it('closes the session and disposes the terminal on unmount', () => {
    const bench = harness();
    const { view } = renderPanel();

    view.unmount();

    expect(bench.session.close).toHaveBeenCalled();
    expect(bench.term.dispose).toHaveBeenCalled();
  });
});

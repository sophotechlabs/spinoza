import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

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

interface Opened {
  pod: string;
  container: string;
}

const opened: Opened[] = [];

vi.mock('../../src/lib/exec', async () => {
  const actual = await vi.importActual<typeof import('../../src/lib/exec')>('../../src/lib/exec');
  return {
    ...actual,
    openExec: (target: Opened) => {
      opened.push(target);
      return { send: vi.fn(), resize: vi.fn(), close: vi.fn() };
    },
    openLocalShell: () => {
      opened.push({ pod: 'this machine', container: 'shell' });
      return { send: vi.fn(), resize: vi.fn(), close: vi.fn() };
    },
  };
});

vi.mock('../../src/components/TerminalPanel', () => ({
  default: ({
    openSession,
    onShellMissing,
  }: {
    openSession: (handlers: { onOutput: () => void; onEnd: () => void }) => unknown;
    onShellMissing: () => void;
  }) => {
    openSession({ onOutput: () => undefined, onEnd: () => undefined });
    const last = opened[opened.length - 1];
    return (
      <div data-testid="terminal-panel">
        {last.pod}/{last.container}
        <button type="button" onClick={onShellMissing}>
          Report missing shell
        </button>
      </div>
    );
  },
}));

import TerminalSession, { LocalTerminalSession } from '../../src/components/TerminalSession';

function stubShell(shell: string): void {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({ shell }) }),
  );
}

function open(container = 'app') {
  return render(<TerminalSession namespace="prod" pod="web" container={container} />);
}

describe('TerminalSession', () => {
  beforeEach(() => {
    stubShell('present');
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('opens a shell into the container it was given', async () => {
    open();

    expect(await screen.findByTestId('terminal-panel')).toHaveTextContent('web/app');
  });

  it('offers a debug container for an image with no shell', async () => {
    stubShell('absent');
    open();

    expect(await screen.findByTestId('debug-prompt')).toHaveTextContent('no shell in app');
  });

  it('opens a shell into the debug container once it is attached', async () => {
    const user = userEvent.setup();
    stubShell('absent');
    open();
    await user.click(await screen.findByRole('button', { name: 'Attach debug container' }));

    expect(await screen.findByTestId('terminal-panel')).toHaveTextContent('web/spinoza-debug-1');
  });

  it('offers a debug container when the session reports no shell', async () => {
    const user = userEvent.setup();
    open();
    await user.click(await screen.findByRole('button', { name: 'Report missing shell' }));

    expect(await screen.findByTestId('debug-prompt')).toBeInTheDocument();
  });

  it('survives a failed support lookup', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));
    open();

    expect(await screen.findByTestId('terminal-panel')).toBeInTheDocument();
  });

  it('says the shell probe failed instead of swallowing it', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));
    open();

    expect(await screen.findByText(/Could not check app for a shell/)).toHaveTextContent('offline');
  });

  it('names no cause when the probe rejects with a non-Error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('nope'));
    open();

    expect(await screen.findByText(/the shell probe failed/)).toBeInTheDocument();
  });

  it('says nothing about the probe once it answers', async () => {
    open();
    await screen.findByTestId('terminal-panel');

    expect(screen.queryByText(/Could not check whether/)).not.toBeInTheDocument();
  });
});

describe('LocalTerminalSession', () => {
  it('opens a shell on this machine without probing a pod', async () => {
    const user = userEvent.setup();
    opened.length = 0;
    render(<LocalTerminalSession />);

    expect(await screen.findByTestId('terminal-panel')).toHaveTextContent('this machine/shell');

    await user.click(screen.getByRole('button', { name: 'Report missing shell' }));

    expect(screen.getByTestId('terminal-panel')).toBeInTheDocument();
  });
});

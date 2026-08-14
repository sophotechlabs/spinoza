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

import TerminalSession from '../../src/components/TerminalSession';

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

    expect(await screen.findByText(/Could not check whether app has a shell/)).toHaveTextContent(
      'offline',
    );
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

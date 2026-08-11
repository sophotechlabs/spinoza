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

import TerminalTab from '../../src/components/TerminalTab';
import { terminalTitle } from '../../src/lib/shell';
import type { PodTarget } from '../../src/lib/pods';

function stubShell(shell: string): void {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({ shell }) }),
  );
}

function pod(overrides: Partial<PodTarget> = {}): PodTarget {
  return { namespace: 'prod', name: 'web', containers: ['app'], ...overrides };
}

describe('TerminalTab', () => {
  beforeEach(() => {
    stubShell('present');
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('prompts for a pod when nothing is selected', () => {
    render(<TerminalTab pod={null} />);

    expect(screen.getByText('Select a pod to open a shell in it.')).toBeInTheDocument();
  });

  it('prompts for a pod that reports no containers', () => {
    render(<TerminalTab pod={pod({ containers: [] })} />);

    expect(screen.getByText('Select a pod to open a shell in it.')).toBeInTheDocument();
  });

  it('opens a shell into the first container', async () => {
    render(<TerminalTab pod={pod()} />);

    expect(await screen.findByTestId('terminal-panel')).toHaveTextContent('web/app');
  });

  it('hides the picker for a single-container pod', () => {
    render(<TerminalTab pod={pod()} />);

    expect(screen.queryByLabelText('Container')).not.toBeInTheDocument();
  });

  it('switches container from the picker', async () => {
    const user = userEvent.setup();
    render(<TerminalTab pod={pod({ containers: ['app', 'sidecar'] })} />);

    await user.selectOptions(screen.getByLabelText('Container'), 'sidecar');

    expect(await screen.findByTestId('terminal-panel')).toHaveTextContent('web/sidecar');
  });

  it('goes back to the first container for the next pod', () => {
    const view = render(<TerminalTab pod={pod({ containers: ['app', 'sidecar'] })} />);

    view.rerender(<TerminalTab pod={pod({ name: 'other', containers: ['app', 'sidecar'] })} />);

    expect(screen.getByLabelText('Container')).toHaveValue('app');
  });

  it('offers a debug container for an image with no shell', async () => {
    stubShell('absent');
    render(<TerminalTab pod={pod()} />);

    expect(await screen.findByTestId('debug-prompt')).toHaveTextContent('no shell in app');
  });

  it('opens a shell into the debug container once it is attached', async () => {
    const user = userEvent.setup();
    stubShell('absent');
    render(<TerminalTab pod={pod()} />);
    await user.click(await screen.findByRole('button', { name: 'Attach debug container' }));

    expect(await screen.findByTestId('terminal-panel')).toHaveTextContent('web/spinoza-debug-1');
  });

  it('offers a debug container when the session reports no shell', async () => {
    const user = userEvent.setup();
    render(<TerminalTab pod={pod()} />);
    await user.click(await screen.findByRole('button', { name: 'Report missing shell' }));

    expect(await screen.findByTestId('debug-prompt')).toBeInTheDocument();
  });

  it('survives a failed support lookup', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));
    render(<TerminalTab pod={pod()} />);

    expect(await screen.findByTestId('terminal-panel')).toBeInTheDocument();
  });

  it('says the shell probe failed instead of swallowing it', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));
    render(<TerminalTab pod={pod()} />);

    expect(await screen.findByText(/Could not check whether app has a shell/)).toHaveTextContent(
      'offline',
    );
  });

  it('names no cause when the probe rejects with a non-Error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('nope'));
    render(<TerminalTab pod={pod()} />);

    expect(await screen.findByText(/the shell probe failed/)).toBeInTheDocument();
  });

  it('says nothing about the probe once it answers', async () => {
    render(<TerminalTab pod={pod()} />);
    await screen.findByTestId('terminal-panel');

    expect(screen.queryByText(/Could not check whether/)).not.toBeInTheDocument();
  });

  it('explains why the terminal is unavailable', () => {
    expect(terminalTitle('absent')).toContain('debug container');
    expect(terminalTitle('present')).toContain('Shell into');
  });
});

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

vi.mock('../../src/components/TerminalSession', () => ({
  default: ({ pod, container }: { pod: string; container: string }) => (
    <div data-testid="terminal-session">
      {pod}/{container}
    </div>
  ),
}));

import TerminalTab from '../../src/components/TerminalTab';
import { terminalTitle } from '../../src/lib/shell';
import type { PodTarget } from '../../src/lib/pods';
import { useTerminalsStore } from '../../src/store/terminals';

function pod(overrides: Partial<PodTarget> = {}): PodTarget {
  return { namespace: 'prod', name: 'web', containers: ['app'], ...overrides };
}

function sessions(): HTMLElement[] {
  return screen.getAllByTestId('terminal-session');
}

function visible(): HTMLElement[] {
  return sessions().filter((session) => session.closest('[hidden]') === null);
}

beforeEach(() => {
  useTerminalsStore.getState().reset();
});

afterEach(() => {
  useTerminalsStore.getState().reset();
});

describe('TerminalTab', () => {
  it('says nothing is open before a shell is asked for', () => {
    render(<TerminalTab pod={null} />);

    expect(screen.getByText(/No shells open/)).toBeInTheDocument();
    expect(screen.queryAllByTestId('terminal-session')).toHaveLength(0);
  });

  it('offers no shell button without a pod to open one in', () => {
    render(<TerminalTab pod={null} />);

    expect(screen.queryByRole('button', { name: /Shell in/ })).not.toBeInTheDocument();
  });

  it('offers no shell button for a pod that reports no containers', () => {
    render(<TerminalTab pod={pod({ containers: [] })} />);

    expect(screen.queryByRole('button', { name: /Shell in/ })).not.toBeInTheDocument();
  });

  it('leaves the selected pod alone until the button is pressed', () => {
    render(<TerminalTab pod={pod()} />);

    expect(screen.getByRole('button', { name: 'Shell in web' })).toBeInTheDocument();
    expect(screen.queryAllByTestId('terminal-session')).toHaveLength(0);
  });

  it('opens a shell into the first container on request', async () => {
    const user = userEvent.setup();
    render(<TerminalTab pod={pod()} />);

    await user.click(screen.getByRole('button', { name: 'Shell in web' }));

    expect(visible()[0]).toHaveTextContent('web/app');
    expect(screen.getByRole('button', { name: 'web/app' })).toHaveAttribute('aria-pressed', 'true');
  });

  it('hides the container picker for a single-container pod', () => {
    render(<TerminalTab pod={pod()} />);

    expect(screen.queryByLabelText('Container')).not.toBeInTheDocument();
  });

  it('opens the container the picker names', async () => {
    const user = userEvent.setup();
    render(<TerminalTab pod={pod({ containers: ['app', 'sidecar'] })} />);

    await user.selectOptions(screen.getByLabelText('Container'), 'sidecar');
    await user.click(screen.getByRole('button', { name: 'Shell in web' }));

    expect(visible()[0]).toHaveTextContent('web/sidecar');
  });

  it('goes back to the first container for the next pod', () => {
    const view = render(<TerminalTab pod={pod({ containers: ['app', 'sidecar'] })} />);

    view.rerender(<TerminalTab pod={pod({ name: 'other', containers: ['app', 'sidecar'] })} />);

    expect(screen.getByLabelText('Container')).toHaveValue('app');
  });

  it('keeps an open shell alive while another pod is selected', async () => {
    const user = userEvent.setup();
    const view = render(<TerminalTab pod={pod()} />);
    await user.click(screen.getByRole('button', { name: 'Shell in web' }));

    view.rerender(<TerminalTab pod={pod({ name: 'db' })} />);

    expect(sessions()[0]).toHaveTextContent('web/app');
    expect(screen.getByRole('button', { name: 'Shell in db' })).toBeInTheDocument();
  });

  it('gives each pod its own tab and shows only the newest', async () => {
    const user = userEvent.setup();
    const view = render(<TerminalTab pod={pod()} />);
    await user.click(screen.getByRole('button', { name: 'Shell in web' }));
    view.rerender(<TerminalTab pod={pod({ name: 'db' })} />);
    await user.click(screen.getByRole('button', { name: 'Shell in db' }));

    expect(sessions()).toHaveLength(2);
    expect(visible()).toHaveLength(1);
    expect(visible()[0]).toHaveTextContent('db/app');
  });

  it('brings a tab back to the front', async () => {
    const user = userEvent.setup();
    const view = render(<TerminalTab pod={pod()} />);
    await user.click(screen.getByRole('button', { name: 'Shell in web' }));
    view.rerender(<TerminalTab pod={pod({ name: 'db' })} />);
    await user.click(screen.getByRole('button', { name: 'Shell in db' }));

    await user.click(screen.getByRole('button', { name: 'web/app' }));

    expect(visible()[0]).toHaveTextContent('web/app');
  });

  it('goes to the shell that is already open instead of a second one', async () => {
    const user = userEvent.setup();
    render(<TerminalTab pod={pod()} />);

    await user.click(screen.getByRole('button', { name: 'Shell in web' }));
    await user.click(screen.getByRole('button', { name: 'Shell in web' }));

    expect(sessions()).toHaveLength(1);
  });

  it('closes a shell from its tab', async () => {
    const user = userEvent.setup();
    render(<TerminalTab pod={pod()} />);
    await user.click(screen.getByRole('button', { name: 'Shell in web' }));

    await user.click(screen.getByRole('button', { name: 'Close the shell in web' }));

    expect(screen.queryAllByTestId('terminal-session')).toHaveLength(0);
    expect(screen.getByText(/No shells open/)).toBeInTheDocument();
  });

  it('explains why the terminal is unavailable', () => {
    expect(terminalTitle('absent')).toContain('debug container');
    expect(terminalTitle('present')).toContain('Shell into');
  });
});

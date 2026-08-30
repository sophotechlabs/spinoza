import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

vi.mock('../../src/components/TerminalSession', () => ({
  default: ({ pod, container }: { pod: string; container: string }) => (
    <div data-testid="terminal-session">
      {pod}/{container}
    </div>
  ),
  LocalTerminalSession: () => <div data-testid="terminal-session">local shell</div>,
  NodeTerminalSession: ({ node }: { node: string }) => (
    <div data-testid="terminal-session">node shell on {node}</div>
  ),
}));

import TerminalTab from '../../src/components/TerminalTab';
import type { PodTarget } from '../../src/lib/pods';
import { useTerminalsStore } from '../../src/store/terminals';
import { accessKey, useAccessStore } from '../../src/store/access';
import { capabilities } from '../helpers';
import { EMPTY_CONTEXTS, useContextsStore } from '../../src/store/contexts';
import { podRef } from '../../src/lib/pods';

function pod(overrides: Partial<PodTarget> = {}): PodTarget {
  return { namespace: 'prod', name: 'web', containers: ['app'], ...overrides };
}

function sessions(): HTMLElement[] {
  return screen.getAllByTestId('terminal-session');
}

function visible(): HTMLElement[] {
  return sessions().filter((session) => session.closest('[hidden]') === null);
}

function stubSupport(available: boolean, reason?: string) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve({
        ok: true,
        json: () => Promise.resolve(capabilities({ localShell: { available, reason } })),
      }),
    ),
  );
}

beforeEach(() => {
  useTerminalsStore.getState().reset();
  stubSupport(false, 'A shell on this machine is only available in the desktop app.');
});

afterEach(() => {
  vi.unstubAllGlobals();
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

  it('says the local shell is desktop-only in a browser tab', async () => {
    render(<TerminalTab pod={null} />);

    expect(
      await screen.findByText('A shell on this machine is only available in the desktop app.'),
    ).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Local shell' })).not.toBeInTheDocument();
  });

  it('opens a shell on this machine by itself in the desktop app', async () => {
    stubSupport(true);

    render(<TerminalTab pod={null} />);

    expect(await screen.findByText('local shell')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'local' })).toHaveAttribute('aria-pressed', 'true');
  });

  it('opens only one shell on this machine, however often it is asked', async () => {
    const user = userEvent.setup();
    stubSupport(true);
    render(<TerminalTab pod={null} />);
    await screen.findByText('local shell');

    await user.click(screen.getByRole('button', { name: 'Local shell' }));

    expect(sessions()).toHaveLength(1);
  });

  it('closes the shell on this machine without opening it again', async () => {
    const user = userEvent.setup();
    stubSupport(true);
    render(<TerminalTab pod={null} />);
    await screen.findByText('local shell');

    await user.click(screen.getByRole('button', { name: 'Close the local shell' }));

    expect(screen.queryAllByTestId('terminal-session')).toHaveLength(0);
    expect(screen.getByText(/No shells open/)).toBeInTheDocument();
  });

  it('keeps pod shells beside the one on this machine', async () => {
    const user = userEvent.setup();
    stubSupport(true);
    render(<TerminalTab pod={pod()} />);
    await screen.findByText('local shell');

    await user.click(screen.getByRole('button', { name: 'Shell in web' }));

    expect(sessions()).toHaveLength(2);
    expect(visible()[0]).toHaveTextContent('web/app');
  });

  it('carries on when the support lookup fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.reject(new Error('offline'))),
    );

    render(<TerminalTab pod={null} />);

    expect(
      await screen.findByText('A shell on this machine is only available in the desktop app.'),
    ).toBeInTheDocument();
  });
});

describe('a node shell tab', () => {
  it('labels the session by its node and offers to close it', async () => {
    useTerminalsStore.getState().openNode('p-mk1');

    render(<TerminalTab pod={null} />);

    expect(await screen.findByRole('button', { name: 'node p-mk1' })).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'Close the shell on node p-mk1' }),
    ).toBeInTheDocument();
    expect(screen.getByTestId('terminal-session')).toHaveTextContent('node shell on p-mk1');
  });
});

describe('a shell the cluster would refuse', () => {
  const podKey = accessKey('p-mk1', podRef(pod()));

  beforeEach(() => {
    useAccessStore.getState().forget();
    useContextsStore.getState().setList({
      ...EMPTY_CONTEXTS,
      current: { kubeconfig: '', name: 'p-mk1' },
    });
  });

  afterEach(() => {
    useAccessStore.getState().forget();
  });

  it('greys out the shell button and says why', () => {
    useAccessStore.getState().setRefused(podKey, {
      exec: 'requires container.pods.exec in Cloud IAM',
    });
    render(<TerminalTab pod={pod()} />);

    const button = screen.getByRole('button', { name: 'Shell in web' });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute('title', 'requires container.pods.exec in Cloud IAM');
  });

  it('leaves the shell button alone when exec is allowed', () => {
    useAccessStore.getState().setRefused(podKey, { logs: 'no logs, but exec is fine' });
    render(<TerminalTab pod={pod()} />);

    expect(screen.getByRole('button', { name: 'Shell in web' })).toBeEnabled();
  });

  it('keeps the refusal for another pod off this one', () => {
    useAccessStore
      .getState()
      .setRefused('group=&version=v1&resource=pods&namespace=prod&name=api', {
        exec: 'not about this pod',
      });
    render(<TerminalTab pod={pod()} />);

    expect(screen.getByRole('button', { name: 'Shell in web' })).toBeEnabled();
  });
});

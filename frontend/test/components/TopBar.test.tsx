import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import TopBar from '../../src/components/TopBar';
import { useClusterHealthStore } from '../../src/store/clusterHealth';
import { useClustersStore } from '../../src/store/clusters';
import { namespaceNow, useNamespaceStore } from '../../src/store/namespace';
import type { ObjectRef } from '../../src/lib/types';
import { notifyOk, useToastsStore } from '../../src/store/toasts';
import { reportHealth } from '../../src/store/clusterHealth';
import { MK1, showing } from '../helpers-clusters';

const podRef: ObjectRef = {
  group: '',
  version: 'v1',
  resource: 'pods',
  namespace: 'prod',
  name: 'web-0',
};

vi.mock('../../src/components/ContextPicker', () => ({
  default: ({ onSwitched }: { onSwitched: () => void }) => (
    <button type="button" onClick={onSwitched}>
      switch context
    </button>
  ),
}));

afterEach(() => {
  useClustersStore.getState().reset();
  useNamespaceStore.getState().reset();
  vi.unstubAllGlobals();
});

function dotFor(container: HTMLElement): Element {
  const dot = container.querySelector('[data-testid="connection-dot"]');
  if (!dot) {
    throw new Error('status dot not found');
  }
  return dot;
}

describe('TopBar', () => {
  it('shows a green dot when connected', () => {
    const { container } = render(<TopBar status="connected" />);
    expect(screen.getByRole('status', { name: 'The cluster feed is connected' })).toBeVisible();
    expect(dotFor(container).className).toContain('bg-ok-solid');
  });

  it('shows a yellow dot when connecting', () => {
    const { container } = render(<TopBar status="connecting" />);
    expect(screen.getByRole('status', { name: 'The cluster feed is connecting' })).toBeVisible();
    expect(dotFor(container).className).toContain('bg-warn-solid');
  });

  it('shows a red dot when disconnected', () => {
    const { container } = render(<TopBar status="disconnected" />);
    expect(screen.getByRole('status', { name: 'The cluster feed is disconnected' })).toBeVisible();
    expect(dotFor(container).className).toContain('bg-error-solid');
  });

  it('says the connection state in a tooltip rather than in words', () => {
    render(<TopBar status="connected" />);

    expect(screen.queryByText('connected')).not.toBeInTheDocument();
    expect(screen.getByRole('status')).toHaveAttribute('title', 'The cluster feed is connected');
  });

  it('calls onReconnect when the reconnect button is clicked', async () => {
    const user = userEvent.setup();
    const onReconnect = vi.fn();
    render(<TopBar status="disconnected" onReconnect={onReconnect} />);
    await user.click(screen.getByRole('button', { name: 'Reconnect' }));
    expect(onReconnect).toHaveBeenCalledTimes(1);
  });

  it('leaves the theme picker to the settings dialog', () => {
    render(<TopBar status="connected" />);
    expect(screen.queryByLabelText('Theme')).not.toBeInTheDocument();
  });

  it('does not throw when reconnect is clicked without a handler', async () => {
    const user = userEvent.setup();
    render(<TopBar status="disconnected" />);
    await user.click(screen.getByRole('button', { name: 'Reconnect' }));
    expect(screen.getByRole('button', { name: 'Reconnect' })).toBeInTheDocument();
  });
});

describe('TopBar context switch', () => {
  it('reports the switch to the owner', async () => {
    const user = userEvent.setup();
    const onContextChanged = vi.fn();
    const onReconnect = vi.fn();
    render(
      <TopBar status="connected" onReconnect={onReconnect} onContextChanged={onContextChanged} />,
    );

    await user.click(screen.getByRole('button', { name: 'switch context' }));

    expect(onContextChanged).toHaveBeenCalledOnce();
    expect(onReconnect).not.toHaveBeenCalled();
  });

  it('falls back to a reconnect when no owner is listening', async () => {
    const user = userEvent.setup();
    const onReconnect = vi.fn();
    render(<TopBar status="connected" onReconnect={onReconnect} />);

    await user.click(screen.getByRole('button', { name: 'switch context' }));

    expect(onReconnect).toHaveBeenCalledOnce();
  });
});

describe('the top bar entry points', () => {
  it('asks the app to open settings from the gear', async () => {
    const user = userEvent.setup();
    const onOpenSettings = vi.fn();
    render(<TopBar status="connected" onOpenSettings={onOpenSettings} />);

    await user.click(screen.getByRole('button', { name: 'Settings' }));

    expect(onOpenSettings).toHaveBeenCalledTimes(1);
  });

  it('names the gear on hover', () => {
    render(<TopBar status="connected" />);

    expect(screen.getByRole('button', { name: 'Settings' })).toHaveAttribute('title', 'Settings');
  });

  it('asks the app to open the palette from the search button', async () => {
    const user = userEvent.setup();
    const onOpenPalette = vi.fn();
    render(<TopBar status="connected" onOpenPalette={onOpenPalette} />);

    await user.click(screen.getByRole('button', { name: /Search/ }));

    expect(onOpenPalette).toHaveBeenCalledTimes(1);
  });

  it('does not blow up when neither handler is wired', async () => {
    const user = userEvent.setup();
    render(<TopBar status="connected" />);

    await user.click(screen.getByRole('button', { name: 'Settings' }));
    await user.click(screen.getByRole('button', { name: /Search/ }));

    expect(screen.getByRole('button', { name: 'Settings' })).toBeInTheDocument();
  });

  it('opens the object behind a notification the bell lists', async () => {
    const user = userEvent.setup();
    const onSelectObject = vi.fn();
    useToastsStore.getState().clear();
    notifyOk('Deleted Pod web-0', podRef);
    render(<TopBar status="connected" onSelectObject={onSelectObject} />);

    await user.click(screen.getByLabelText('Notifications'));
    await user.click(screen.getByRole('button', { name: 'pods/prod/web-0' }));

    expect(onSelectObject).toHaveBeenCalledWith(podRef);
  });

  it('takes a notification click quietly when nothing is wired to it', async () => {
    const user = userEvent.setup();
    useToastsStore.getState().clear();
    notifyOk('Deleted Pod web-0', podRef);
    render(<TopBar status="connected" />);

    await user.click(screen.getByLabelText('Notifications'));
    await user.click(screen.getByRole('button', { name: 'pods/prod/web-0' }));

    expect(screen.getByLabelText('Notifications')).toBeInTheDocument();
  });

  it('tells the app when the window took spinoza back', async () => {
    const user = userEvent.setup();
    const onLeftForDesktop = vi.fn();
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/api/view/')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () => Promise.resolve({ switched: true }),
          });
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({ window: true, hidden: true }),
        });
      }),
    );
    render(<TopBar status="connected" onLeftForDesktop={onLeftForDesktop} />);

    await user.click(await screen.findByRole('button', { name: 'Desktop' }));

    await waitFor(() => {
      expect(onLeftForDesktop).toHaveBeenCalledTimes(1);
    });
    vi.unstubAllGlobals();
  });

  it('takes the switch back quietly when nothing is wired to it', async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/api/view/')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () => Promise.resolve({ switched: true }),
          });
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({ window: true, hidden: true }),
        });
      }),
    );
    render(<TopBar status="connected" />);

    await user.click(await screen.findByRole('button', { name: 'Desktop' }));

    expect(await screen.findByRole('button', { name: 'Desktop' })).toBeInTheDocument();
    vi.unstubAllGlobals();
  });

  it('offers reconnect as an icon with a tooltip', async () => {
    const user = userEvent.setup();
    const onReconnect = vi.fn();
    render(<TopBar status="disconnected" onReconnect={onReconnect} />);

    const button = screen.getByRole('button', { name: 'Reconnect' });
    expect(button).toHaveAttribute('title', 'Reconnect to the cluster');
    expect(button.textContent).toBe('');

    await user.click(button);

    expect(onReconnect).toHaveBeenCalledTimes(1);
  });

  it('keeps the connection state beside the cluster, not off to the right', () => {
    const { container } = render(<TopBar status="connected" />);

    const bar = container.querySelector('header');
    const dot = dotFor(container);
    const search = screen.getByRole('button', { name: /Search/ });
    expect(bar).not.toBeNull();
    expect(dot.compareDocumentPosition(search) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it('offers the namespaces the cluster reported', async () => {
    showing(MK1);
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({ names: ['argocd', 'default', 'kube-system'] }),
        }),
      ),
    );

    render(<TopBar status="connected" />);

    const picker = await screen.findByRole('combobox', { name: 'Namespace' });
    expect(within(picker).getByRole('option', { name: 'All namespaces' })).toBeInTheDocument();
    await waitFor(() => {
      expect(within(picker).getByRole('option', { name: 'kube-system' })).toBeInTheDocument();
    });
    expect(picker).toHaveValue('');
  });

  it('takes the namespace that was chosen', async () => {
    const user = userEvent.setup();
    showing(MK1);
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({ names: ['default', 'shop'] }),
        }),
      ),
    );
    render(<TopBar status="connected" />);
    const picker = await screen.findByRole('combobox', { name: 'Namespace' });
    await waitFor(() => {
      expect(within(picker).getByRole('option', { name: 'shop' })).toBeInTheDocument();
    });

    await user.selectOptions(picker, 'shop');

    expect(namespaceNow()).toBe('shop');
  });

  it('stays open while nothing says the kind is cluster-scoped', () => {
    render(<TopBar status="connected" />);

    const picker = screen.getByRole('combobox', { name: 'Namespace' });
    expect(picker).toBeEnabled();
    expect(picker).toHaveAttribute('title', 'The namespace the resource list shows');
  });

  it('shuts for a kind that is not in a namespace', () => {
    render(<TopBar status="connected" scoped={false} />);

    const picker = screen.getByRole('combobox', { name: 'Namespace' });
    expect(picker).toBeDisabled();
    expect(picker).toHaveAttribute('title', 'This kind is not in a namespace');
  });
});

describe('a cluster that stopped answering', () => {
  beforeEach(() => {
    useClusterHealthStore.getState().reset();
  });

  afterEach(() => {
    useClusterHealthStore.getState().reset();
  });

  it('turns the dot red even though the feed is connected', () => {
    reportHealth('', false, false, 'connection refused');

    const { container } = render(<TopBar status="connected" />);

    expect(dotFor(container).className).toContain('bg-error-solid');
  });

  it('says so in words, not only in colour', () => {
    reportHealth('', false, false, 'connection refused');

    render(<TopBar status="connected" />);

    expect(screen.getByText('cluster not answering')).toBeVisible();
  });

  it('carries the reason the cluster gave', () => {
    reportHealth('', false, false, 'dial tcp 10.0.0.1:6443: connection refused');

    render(<TopBar status="connected" />);

    expect(
      screen.getByRole('status', {
        name: 'The cluster is not answering: dial tcp 10.0.0.1:6443: connection refused',
      }),
    ).toBeVisible();
  });

  it('still says something useful when no reason came with it', () => {
    reportHealth('', false, false, '');

    render(<TopBar status="connected" />);

    expect(
      screen.getByRole('status', {
        name: 'The cluster is not answering; what is on screen is the last thing it said',
      }),
    ).toBeVisible();
  });

  it('goes back to green when the cluster answers again', () => {
    reportHealth('', false, false, 'connection refused');
    const { container, rerender } = render(<TopBar status="connected" />);
    expect(dotFor(container).className).toContain('bg-error-solid');

    act(() => {
      reportHealth('', true, false, '');
    });
    rerender(<TopBar status="connected" />);

    expect(dotFor(container).className).toContain('bg-ok-solid');
    expect(screen.queryByText('cluster not answering')).not.toBeInTheDocument();
  });

  it('leaves a disconnected feed reading as disconnected', () => {
    reportHealth('', false, false, 'connection refused');

    render(<TopBar status="disconnected" />);

    expect(screen.getByRole('status', { name: 'The cluster feed is disconnected' })).toBeVisible();
    expect(screen.queryByText('cluster not answering')).not.toBeInTheDocument();
  });
});

describe('a cluster that missed a ping', () => {
  beforeEach(() => {
    useClusterHealthStore.getState().reset();
  });

  afterEach(() => {
    useClusterHealthStore.getState().reset();
  });

  it('turns the dot amber, not red', () => {
    reportHealth('', true, true, 'i/o timeout');

    const { container } = render(<TopBar status="connected" />);

    expect(dotFor(container).className).toContain('bg-warn-solid');
    expect(dotFor(container).className).not.toContain('bg-error-solid');
  });

  it('says a ping was missed rather than that the cluster is gone', () => {
    reportHealth('', true, true, 'i/o timeout');

    render(<TopBar status="connected" />);

    expect(screen.getByRole('status').getAttribute('title')).toContain('missed a ping');
    expect(screen.getByRole('status').getAttribute('title')).toContain('i/o timeout');
  });

  it('says so without a reason when the cluster gave none', () => {
    reportHealth('', true, true, '');

    render(<TopBar status="connected" />);

    expect(screen.getByRole('status').getAttribute('title')).toContain(
      'still showing what it last said',
    );
  });

  it('is green again once the cluster answers', () => {
    reportHealth('', true, false, '');

    const { container } = render(<TopBar status="connected" />);

    expect(dotFor(container).className).toContain('bg-ok-solid');
  });
});

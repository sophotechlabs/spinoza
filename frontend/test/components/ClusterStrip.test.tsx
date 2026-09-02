import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ClusterStrip from '../../src/components/ClusterStrip';
import { adoptClusters, useClustersStore } from '../../src/store/clusters';
import { reportHealth, useClusterHealthStore } from '../../src/store/clusterHealth';
import { rememberObject, useRecentsStore } from '../../src/store/recents';
import { useTerminalsStore } from '../../src/store/terminals';
import { setForwards, useForwardsStore } from '../../src/store/forwards';
import { useToastsStore } from '../../src/store/toasts';
import { MK1, MK2, listOf } from '../helpers-clusters';

interface Call {
  url: string;
  method: string;
}

function stub(ok = true, body: unknown = listOf(MK2)): Call[] {
  const calls: Call[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: { method?: string }) => {
      calls.push({ url, method: init?.method ?? 'GET' });
      return Promise.resolve({
        ok,
        status: 500,
        json: () => Promise.resolve(body),
      });
    }),
  );
  return calls;
}

function open(active: string): void {
  act(() => {
    adoptClusters(listOf(active));
  });
}

describe('the strip of open clusters', () => {
  beforeEach(() => {
    useClustersStore.getState().reset();
    useClusterHealthStore.getState().reset();
    useTerminalsStore.getState().reset();
    useForwardsStore.getState().clear();
    useToastsStore.setState({ toasts: [], history: [] });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('stays out of the way while one cluster is open', () => {
    act(() => {
      adoptClusters({ clusters: listOf(MK1).clusters.slice(0, 1), remembered: [] });
    });

    render(<ClusterStrip onShown={vi.fn()} />);

    expect(screen.queryByRole('navigation')).not.toBeInTheDocument();
  });

  it('names every open cluster once a second one is open', () => {
    open(MK1);

    render(<ClusterStrip onShown={vi.fn()} />);

    expect(screen.getByRole('button', { name: 'p-mk1' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'p-mk2' })).toBeInTheDocument();
  });

  it('marks the tab in front', () => {
    open(MK2);

    render(<ClusterStrip onShown={vi.fn()} />);

    expect(screen.getByRole('button', { name: 'p-mk2' })).toHaveAttribute('aria-current', 'true');
    expect(screen.getByRole('button', { name: 'p-mk1' })).toHaveAttribute('aria-current', 'false');
  });

  it('says which tabs are answering', () => {
    open(MK1);
    act(() => {
      reportHealth(MK2, false, false, 'no route to host');
    });

    render(<ClusterStrip onShown={vi.fn()} />);

    const down = screen.getByRole('button', { name: /p-mk2 is not answering/ });
    expect(down).toHaveAttribute('title', 'no route to host');
    expect(screen.getByRole('button', { name: /p-mk1 is answering/ })).toBeInTheDocument();
  });

  it('brings a tab forward when it is clicked', async () => {
    const user = userEvent.setup();
    const calls = stub();
    open(MK1);
    const onShown = vi.fn();
    render(<ClusterStrip onShown={onShown} />);

    await user.click(screen.getByRole('button', { name: 'p-mk2' }));

    expect(onShown).toHaveBeenCalled();
    await waitFor(() => {
      expect(calls.some((call) => call.url.includes('/api/clusters/active'))).toBe(true);
    });
  });

  it('does not overlap cluster switches', async () => {
    const list = listOf(MK1);
    list.clusters.push({
      id: 'https://p-mk3:6443',
      context: 'p-mk3',
      active: false,
      color: 3,
      reopen: true,
      protection: 'open',
      reachable: true,
    });
    let finishSwitch: (response: unknown) => void = () => undefined;
    const switching = new Promise((resolve) => {
      finishSwitch = resolve;
    });
    const fetchMock = vi.fn(() => switching);
    vi.stubGlobal('fetch', fetchMock);
    act(() => {
      adoptClusters(list);
    });
    render(<ClusterStrip onShown={vi.fn()} />);

    fireEvent.click(screen.getByRole('button', { name: 'p-mk2' }));
    fireEvent.click(screen.getByRole('button', { name: 'p-mk3' }));

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('button', { name: 'p-mk3' })).toBeDisabled();
    await act(async () => {
      finishSwitch({ ok: true, status: 200, json: () => Promise.resolve(list) });
      await switching;
      await Promise.resolve();
    });

    expect(screen.getByRole('button', { name: 'p-mk3' })).toBeEnabled();
  });

  it('does nothing when the tab in front is clicked', async () => {
    const user = userEvent.setup();
    const calls = stub();
    open(MK1);
    render(<ClusterStrip onShown={vi.fn()} />);

    await user.click(screen.getByRole('button', { name: 'p-mk1' }));

    expect(calls).toHaveLength(0);
  });

  it('says so when the server will not bring a tab forward', async () => {
    const user = userEvent.setup();
    stub(false, { message: 'no route to host' });
    open(MK1);
    render(<ClusterStrip onShown={vi.fn()} />);

    await user.click(screen.getByRole('button', { name: 'p-mk2' }));

    await waitFor(() => {
      expect(useToastsStore.getState().toasts[0].message).toContain('Switching to p-mk2');
    });
    expect(useClustersStore.getState().active).toBe(MK1);
  });

  it('closes a tab and lets go of what belonged to it', async () => {
    const user = userEvent.setup();
    stub(true, { clusters: listOf(MK1).clusters.slice(0, 1), remembered: [] });
    open(MK1);
    act(() => {
      rememberObject({
        group: '',
        version: 'v1',
        resource: 'pods',
        namespace: 'prod',
        name: 'web',
      });
    });
    render(<ClusterStrip onShown={vi.fn()} />);

    await user.click(screen.getByRole('button', { name: 'Close p-mk1' }));

    await waitFor(() => {
      expect(useRecentsStore.getState().byCluster[MK1]).toBeUndefined();
    });
  });

  it('wears the colour the server gave it', () => {
    open(MK1);

    render(<ClusterStrip onShown={vi.fn()} />);

    const swatch = screen.getByRole('button', { name: /p-mk2 is answering/ });
    expect(swatch).toHaveStyle({ backgroundColor: 'var(--cluster-2)' });
  });

  it('opens the tab settings when the swatch is clicked', async () => {
    const user = userEvent.setup();
    open(MK1);
    render(<ClusterStrip onShown={vi.fn()} />);

    await user.click(screen.getByRole('button', { name: /p-mk1 is answering/ }));

    expect(screen.getByRole('group', { name: 'Settings for p-mk1' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Colour 8' })).toBeInTheDocument();
  });

  it('puts the settings away when the swatch is clicked again', async () => {
    const user = userEvent.setup();
    open(MK1);
    render(<ClusterStrip onShown={vi.fn()} />);
    await user.click(screen.getByRole('button', { name: /p-mk1 is answering/ }));

    await user.click(screen.getByRole('button', { name: /p-mk1 is answering/ }));

    expect(screen.queryByRole('group', { name: 'Settings for p-mk1' })).not.toBeInTheDocument();
  });

  it('puts the settings away when you click elsewhere', async () => {
    const user = userEvent.setup();
    open(MK1);
    render(<ClusterStrip onShown={vi.fn()} />);
    await user.click(screen.getByRole('button', { name: /p-mk1 is answering/ }));

    await user.click(document.body);

    expect(screen.queryByRole('group', { name: 'Settings for p-mk1' })).not.toBeInTheDocument();
  });

  it('asks the server for the colour that was picked', async () => {
    const user = userEvent.setup();
    const calls = stub();
    open(MK1);
    render(<ClusterStrip onShown={vi.fn()} />);
    await user.click(screen.getByRole('button', { name: /p-mk1 is answering/ }));

    await user.click(screen.getByRole('button', { name: 'Colour 6' }));

    await waitFor(() => {
      expect(calls.some((call) => call.url.includes('color=6'))).toBe(true);
    });
  });

  it('says so when the colour will not stick', async () => {
    const user = userEvent.setup();
    stub(false, { message: 'read-only file system' });
    open(MK1);
    render(<ClusterStrip onShown={vi.fn()} />);
    await user.click(screen.getByRole('button', { name: /p-mk1 is answering/ }));

    await user.click(screen.getByRole('button', { name: 'Colour 6' }));

    await waitFor(() => {
      expect(useToastsStore.getState().toasts[0].message).toContain('Recolouring p-mk1');
    });
  });

  it('closes the settings once a name has been saved', async () => {
    const user = userEvent.setup();
    stub(true, listOf(MK1));
    open(MK1);
    render(<ClusterStrip onShown={vi.fn()} />);
    await user.click(screen.getByRole('button', { name: /p-mk1 is answering/ }));

    await user.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => {
      expect(screen.queryByRole('group', { name: 'Settings for p-mk1' })).not.toBeInTheDocument();
    });
  });

  it('groups tabs that were put in a group, and names the run', () => {
    act(() => {
      adoptClusters({
        clusters: [
          {
            id: MK1,
            context: 'p-mk1',
            active: true,
            color: 1,
            grouping: 'Client A',
            reopen: true,
            protection: 'open',
            reachable: true,
          },
          {
            id: MK2,
            context: 'p-mk2',
            active: false,
            color: 2,
            reopen: true,
            protection: 'open',
            reachable: true,
          },
        ],
        remembered: [],
      });
    });

    render(<ClusterStrip onShown={vi.fn()} />);

    expect(screen.getByText('Client A')).toBeInTheDocument();
  });

  it('calls a tab by the name it was given', () => {
    act(() => {
      adoptClusters({
        clusters: [
          {
            id: MK1,
            context: 'p-mk1',
            active: true,
            color: 1,
            label: 'client a prod',
            reopen: true,
            protection: 'open',
            reachable: true,
          },
          {
            id: MK2,
            context: 'p-mk2',
            active: false,
            color: 2,
            reopen: true,
            protection: 'open',
            reachable: true,
          },
        ],
        remembered: [],
      });
    });

    render(<ClusterStrip onShown={vi.fn()} />);

    expect(screen.getByRole('button', { name: 'client a prod' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Close client a prod' })).toBeInTheDocument();
  });

  it('asks before closing a tab with a shell attached', async () => {
    const user = userEvent.setup();
    const calls = stub();
    open(MK1);
    act(() => {
      useTerminalsStore.getState().open('prod', 'web', 'app');
    });
    render(<ClusterStrip onShown={vi.fn()} />);

    await user.click(screen.getByRole('button', { name: 'Close p-mk1' }));

    expect(screen.getByText(/1 shell/)).toBeInTheDocument();
    expect(calls).toHaveLength(0);
  });

  it('names every kind of thing that is still attached', async () => {
    const user = userEvent.setup();
    stub();
    open(MK1);
    act(() => {
      useTerminalsStore.getState().open('prod', 'web', 'app');
      useTerminalsStore.getState().open('prod', 'api', 'app');
      setForwards([
        {
          id: '1',
          kind: 'pods',
          namespace: 'prod',
          name: 'web',
          localPort: 8080,
          remotePort: 80,
          state: 'running',
          startedAt: '2026-08-29T12:00:00Z',
        },
      ]);
    });
    render(<ClusterStrip onShown={vi.fn()} />);

    await user.click(screen.getByRole('button', { name: 'Close p-mk1' }));

    expect(screen.getByText(/2 shells and 1 port-forward/)).toBeInTheDocument();
  });

  it('keeps the tab open when you say no', async () => {
    const user = userEvent.setup();
    const calls = stub();
    open(MK1);
    act(() => {
      useTerminalsStore.getState().open('prod', 'web', 'app');
    });
    render(<ClusterStrip onShown={vi.fn()} />);
    await user.click(screen.getByRole('button', { name: 'Close p-mk1' }));

    await user.click(screen.getByRole('button', { name: 'Keep it open' }));

    expect(calls).toHaveLength(0);
    expect(screen.queryByText(/still has/)).not.toBeInTheDocument();
  });

  it('closes it once you say so', async () => {
    const user = userEvent.setup();
    const calls = stub(true, { clusters: listOf(MK1).clusters.slice(1), remembered: [] });
    open(MK1);
    act(() => {
      useTerminalsStore.getState().open('prod', 'web', 'app');
    });
    render(<ClusterStrip onShown={vi.fn()} />);
    await user.click(screen.getByRole('button', { name: 'Close p-mk1' }));

    await user.click(screen.getByRole('button', { name: 'Close it' }));

    await waitFor(() => {
      expect(calls.some((call) => call.method === 'DELETE')).toBe(true);
    });
  });

  it('says so when the server will not close a tab', async () => {
    const user = userEvent.setup();
    stub(false, { message: 'a shell is still attached' });
    open(MK1);
    render(<ClusterStrip onShown={vi.fn()} />);

    await user.click(screen.getByRole('button', { name: 'Close p-mk2' }));

    await waitFor(() => {
      expect(useToastsStore.getState().toasts[0].message).toContain('Closing p-mk2');
    });
  });

  it('scrolls rather than wrapping once the tabs stop fitting', () => {
    open(MK1);

    render(<ClusterStrip onShown={vi.fn()} />);

    expect(screen.getByRole('navigation')).toHaveClass('overflow-x-auto');
  });
});

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ClusterStrip from '../../src/components/ClusterStrip';
import { adoptClusters, useClustersStore } from '../../src/store/clusters';
import { reportHealth, useClusterHealthStore } from '../../src/store/clusterHealth';
import { rememberObject, useRecentsStore } from '../../src/store/recents';
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
      reportHealth(MK2, false, 'no route to host');
    });

    render(<ClusterStrip onShown={vi.fn()} />);

    expect(screen.getByLabelText('not answering')).toHaveAttribute('title', 'no route to host');
    expect(screen.getByLabelText('answering')).toBeInTheDocument();
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
});

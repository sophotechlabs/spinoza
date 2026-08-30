import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import TabMenu from '../../src/components/TabMenu';
import { adoptClusters, useClustersStore } from '../../src/store/clusters';
import type { Tab } from '../../src/store/clusters';
import { reportHealth, useClusterHealthStore } from '../../src/store/clusterHealth';
import { rememberObject, useRecentsStore } from '../../src/store/recents';
import { useToastsStore } from '../../src/store/toasts';
import { MK1, MK2, listOf } from '../helpers-clusters';

interface Call {
  url: string;
  method: string;
}

function stub(ok = true, body: unknown = listOf(MK1)): Call[] {
  const calls: Call[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: { method?: string }) => {
      calls.push({ url, method: init?.method ?? 'GET' });
      return Promise.resolve({ ok, status: 500, json: () => Promise.resolve(body) });
    }),
  );
  return calls;
}

function tabOf(overrides: Partial<Tab> = {}): Tab {
  return {
    id: MK1,
    context: 'p-mk1',
    kubeconfig: '/work.yaml',
    color: 1,
    label: '',
    grouping: '',
    reopen: true,
    timeline: '',
    protection: 'open',
    ...overrides,
  };
}

describe('the settings on a tab', () => {
  beforeEach(() => {
    useClustersStore.getState().reset();
    useClusterHealthStore.getState().reset();
    useRecentsStore.getState().clear();
    useToastsStore.setState({ toasts: [], history: [] });
    act(() => {
      adoptClusters(listOf(MK1));
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('marks the colour the tab is wearing', () => {
    render(<TabMenu tab={tabOf({ color: 3 })} onDone={vi.fn()} />);

    expect(screen.getByRole('button', { name: 'Colour 3' })).toHaveAttribute(
      'aria-current',
      'true',
    );
  });

  it('asks the server for the colour that was picked', async () => {
    const user = userEvent.setup();
    const calls = stub();
    render(<TabMenu tab={tabOf()} onDone={vi.fn()} />);

    await user.click(screen.getByRole('button', { name: 'Colour 6' }));

    await waitFor(() => {
      expect(calls.some((call) => call.url.includes('color=6'))).toBe(true);
    });
  });

  it('offers the context name as the placeholder until one is typed', () => {
    render(<TabMenu tab={tabOf()} onDone={vi.fn()} />);

    expect(screen.getByLabelText('Name')).toHaveAttribute('placeholder', 'p-mk1');
  });

  it('shows the name a tab already has', () => {
    render(
      <TabMenu tab={tabOf({ label: 'client a prod', grouping: 'Client A' })} onDone={vi.fn()} />,
    );

    expect(screen.getByLabelText('Name')).toHaveValue('client a prod');
    expect(screen.getByLabelText('Group')).toHaveValue('Client A');
  });

  it('sends the name and the group together, and closes', async () => {
    const user = userEvent.setup();
    const calls = stub();
    const onDone = vi.fn();
    render(<TabMenu tab={tabOf()} onDone={onDone} />);

    await user.type(screen.getByLabelText('Name'), 'prod');
    await user.type(screen.getByLabelText('Group'), 'Client A');
    await user.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => {
      expect(onDone).toHaveBeenCalled();
    });
    const sent = calls.find((call) => call.url.includes('/api/clusters/name'));
    expect(sent?.url).toContain('label=prod');
    expect(sent?.url).toContain('grouping=Client+A');
  });

  it('says so when a name will not stick', async () => {
    const user = userEvent.setup();
    stub(false, { message: 'read-only file system' });
    render(<TabMenu tab={tabOf()} onDone={vi.fn()} />);

    await user.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => {
      expect(useToastsStore.getState().toasts[0].message).toContain('Renaming p-mk1');
    });
  });

  it('names the tab by its own label once it has one', async () => {
    const user = userEvent.setup();
    stub(false, { message: 'nope' });
    render(<TabMenu tab={tabOf({ label: 'client a prod' })} onDone={vi.fn()} />);

    await user.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => {
      expect(useToastsStore.getState().toasts[0].message).toContain('client a prod');
    });
  });

  it('remembers the tab for next time, or stops', async () => {
    const user = userEvent.setup();
    const calls = stub();
    render(<TabMenu tab={tabOf()} onDone={vi.fn()} />);

    await user.click(screen.getByLabelText(/Open this cluster again/));

    await waitFor(() => {
      expect(calls.some((call) => call.url.includes('reopen=false'))).toBe(true);
    });
  });

  it('turns it back on for a tab that was told not to come back', async () => {
    const user = userEvent.setup();
    const calls = stub();
    render(<TabMenu tab={tabOf({ reopen: false })} onDone={vi.fn()} />);

    await user.click(screen.getByLabelText(/Open this cluster again/));

    await waitFor(() => {
      expect(calls.some((call) => call.url.includes('reopen=true'))).toBe(true);
    });
  });

  it('offers no reconnect while the cluster is answering', () => {
    render(<TabMenu tab={tabOf()} onDone={vi.fn()} />);

    expect(screen.queryByRole('button', { name: 'Reconnect' })).not.toBeInTheDocument();
  });

  it('offers a reconnect once the cluster stops answering', () => {
    act(() => {
      reportHealth(MK1, false, false, 'no route to host');
    });

    render(<TabMenu tab={tabOf()} onDone={vi.fn()} />);

    expect(screen.getByRole('button', { name: 'Reconnect' })).toBeInTheDocument();
  });

  it('reconnects by closing the cluster and opening it again', async () => {
    const user = userEvent.setup();
    const calls = stub();
    act(() => {
      reportHealth(MK1, false, false, 'gone');
      rememberObject({ group: '', version: 'v1', resource: 'pods', namespace: 'p', name: 'web' });
    });
    render(<TabMenu tab={tabOf()} onDone={vi.fn()} />);

    await user.click(screen.getByRole('button', { name: 'Reconnect' }));

    await waitFor(() => {
      expect(calls.some((call) => call.method === 'DELETE')).toBe(true);
    });
    await waitFor(() => {
      expect(calls.some((call) => call.url.includes('name=p-mk1'))).toBe(true);
    });
    expect(useRecentsStore.getState().byCluster[MK1]).toBeUndefined();
  });

  it('says so when a reconnect fails', async () => {
    const user = userEvent.setup();
    stub(false, { message: 'no route to host' });
    act(() => {
      reportHealth(MK1, false, false, 'gone');
    });
    render(<TabMenu tab={tabOf()} onDone={vi.fn()} />);

    await user.click(screen.getByRole('button', { name: 'Reconnect' }));

    await waitFor(() => {
      expect(useToastsStore.getState().toasts[0].message).toContain('Reconnecting to p-mk1');
    });
  });

  it("leaves another tab's health out of it", () => {
    act(() => {
      reportHealth(MK2, false, false, 'gone');
    });

    render(<TabMenu tab={tabOf()} onDone={vi.fn()} />);

    expect(screen.queryByRole('button', { name: 'Reconnect' })).not.toBeInTheDocument();
  });
});

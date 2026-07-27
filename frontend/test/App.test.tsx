import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { Category } from '../src/lib/types';

const feedMocks = vi.hoisted(() => ({
  subscribe: vi.fn(),
  unsubscribe: vi.fn(),
  reconnect: vi.fn(),
}));

vi.mock('../src/lib/feed', () => ({
  useResourceFeed: () => ({
    status: 'connected',
    subscribe: feedMocks.subscribe,
    unsubscribe: feedMocks.unsubscribe,
    reconnect: feedMocks.reconnect,
  }),
}));

interface GraphNodeStub {
  id: string;
  kind: string;
  group: string;
  name: string;
  namespace: string;
  status: string;
  category: string;
}

const graphMocks = vi.hoisted<{ node: GraphNodeStub }>(() => ({
  node: {
    id: 'gn-1',
    kind: 'HelmRelease',
    group: 'helm.toolkit.fluxcd.io',
    name: 'podinfo',
    namespace: 'apps',
    status: 'Ready',
    category: 'app',
  },
}));

vi.mock('../src/components/GitopsGraph', () => ({
  default: ({ onSelect }: { onSelect?: (node: GraphNodeStub) => void }) => (
    <div data-testid="gitops-graph">
      <button
        type="button"
        onClick={() => {
          if (onSelect) {
            onSelect(graphMocks.node);
          }
        }}
      >
        select-node
      </button>
    </div>
  ),
}));

vi.mock('../src/components/FluxDashboard', () => ({
  default: () => <div data-testid="flux-dashboard" />,
}));

vi.mock('../src/components/FluxTiles', () => ({
  default: () => <div data-testid="flux-tiles" />,
}));

vi.mock('../src/components/FluxResources', () => ({
  default: () => <div data-testid="flux-resources" />,
}));

import App from '../src/App';
import { useResourcesStore } from '../src/store/resources';
import { makeCategory, makeColumns, makeDescriptor, makeRow } from './helpers';

const podDescriptor = makeDescriptor({
  group: '',
  version: 'v1',
  resource: 'pods',
  kind: 'Pod',
  namespaced: true,
});

const deploymentDescriptor = makeDescriptor({
  group: 'apps',
  version: 'v1',
  resource: 'deployments',
  kind: 'Deployment',
  namespaced: true,
});

const categories: Category[] = [makeCategory('Workloads', [podDescriptor, deploymentDescriptor])];

function stubFetch(): void {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string) => {
      if (url === '/api/metrics') {
        return Promise.resolve({ ok: true, json: () => Promise.resolve({ pods: {}, nodes: {} }) });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve(categories) });
    }),
  );
}

function resetStore(): void {
  useResourcesStore.setState({ subs: new Map() });
}

async function expandWorkloads(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await user.click(await screen.findByRole('button', { name: /Workloads/ }));
}

describe('App', () => {
  beforeEach(() => {
    resetStore();
    stubFetch();
    feedMocks.subscribe.mockClear();
    feedMocks.unsubscribe.mockClear();
    feedMocks.reconnect.mockClear();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    resetStore();
  });

  it('renders the connection status and placeholders before a resource is chosen', () => {
    render(<App />);
    expect(screen.getByText('connected')).toBeInTheDocument();
    expect(screen.getByText('Select a resource to view.')).toBeInTheDocument();
    expect(screen.getByText('Select a row to see details.')).toBeInTheDocument();
  });

  it('subscribes and renders rows when a resource is selected', async () => {
    useResourcesStore
      .getState()
      .applySnapshot('main', makeColumns(['Ready']), true, [
        makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod', cells: ['1/1'] }),
      ]);
    const user = userEvent.setup();
    render(<App />);
    await expandWorkloads(user);
    await user.click(await screen.findByRole('button', { name: 'Pod' }));
    expect(feedMocks.subscribe).toHaveBeenCalledWith('main', podDescriptor, '');
    expect(await screen.findByRole('button', { name: 'pod-a' })).toBeInTheDocument();
    expect(screen.getByText('1/1')).toBeInTheDocument();
  });

  it('opens and closes the details drawer when a row is selected', async () => {
    useResourcesStore
      .getState()
      .applySnapshot('main', makeColumns([]), true, [
        makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' }),
      ]);
    const user = userEvent.setup();
    render(<App />);
    await expandWorkloads(user);
    await user.click(await screen.findByRole('button', { name: 'Pod' }));
    await user.click(await screen.findByRole('button', { name: 'pod-a' }));
    const drawer = screen.getByRole('complementary');
    expect(within(drawer).getAllByText('pod-a')).toHaveLength(2);
    await user.click(screen.getByRole('button', { name: 'Close' }));
    expect(screen.getByText('Select a row to see details.')).toBeInTheDocument();
  });

  it('unsubscribes the previous resource when switching resources', async () => {
    const user = userEvent.setup();
    render(<App />);
    await expandWorkloads(user);
    await user.click(await screen.findByRole('button', { name: 'Pod' }));
    feedMocks.unsubscribe.mockClear();
    await user.click(screen.getByRole('button', { name: 'Deployment' }));
    expect(feedMocks.unsubscribe).toHaveBeenCalledWith('main');
    expect(feedMocks.subscribe).toHaveBeenCalledWith('main', deploymentDescriptor, '');
  });

  it('reconnects when the reconnect button is clicked', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole('button', { name: 'Reconnect' }));
    expect(feedMocks.reconnect).toHaveBeenCalledTimes(1);
  });

  it('toggles the bottom dock panel', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole('button', { name: /Panel/ }));
    expect(screen.getByText('No output.')).toBeInTheDocument();
  });

  it('switches to the gitops view and shows the graph', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole('button', { name: 'Graph' }));
    expect(screen.getByTestId('gitops-graph')).toBeInTheDocument();
    expect(screen.getByText('Select a node to see details.')).toBeInTheDocument();
    expect(screen.queryByText('Select a row to see details.')).not.toBeInTheDocument();
  });

  it('shows node details when a graph node is selected', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole('button', { name: 'Graph' }));
    await user.click(screen.getByRole('button', { name: 'select-node' }));
    expect(screen.getByText('podinfo')).toBeInTheDocument();
    expect(screen.getByText('HelmRelease')).toBeInTheDocument();
  });

  it('clears the selected node when the node panel is closed', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole('button', { name: 'Graph' }));
    await user.click(screen.getByRole('button', { name: 'select-node' }));
    expect(screen.getByText('podinfo')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Close' }));
    expect(screen.getByText('Select a node to see details.')).toBeInTheDocument();
  });

  it('switches to the flux view and shows the dashboard', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole('button', { name: 'Flux' }));
    expect(screen.getByTestId('flux-dashboard')).toBeInTheDocument();
    expect(screen.queryByText('Select a row to see details.')).not.toBeInTheDocument();
    expect(screen.queryByText('Select a node to see details.')).not.toBeInTheDocument();
  });

  it('switches to the flux tiles view', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole('button', { name: 'Flux Dashboard' }));
    expect(screen.getByTestId('flux-tiles')).toBeInTheDocument();
    expect(screen.queryByText('Select a row to see details.')).not.toBeInTheDocument();
  });

  it('switches to the flux resources overview', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole('button', { name: 'Overview' }));
    expect(screen.getByTestId('flux-resources')).toBeInTheDocument();
    expect(screen.queryByText('Select a row to see details.')).not.toBeInTheDocument();
  });

  it('returns to the resources view when a resource is selected', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole('button', { name: 'Graph' }));
    expect(screen.getByTestId('gitops-graph')).toBeInTheDocument();
    await expandWorkloads(user);
    await user.click(await screen.findByRole('button', { name: 'Pod' }));
    expect(screen.queryByTestId('gitops-graph')).not.toBeInTheDocument();
    expect(screen.getByText('Select a row to see details.')).toBeInTheDocument();
  });
});

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { Category, FluxResource, GraphNode, ObjectRef } from '../src/lib/types';

const feedMocks = vi.hoisted(() => ({
  subscribe: vi.fn(),
  unsubscribe: vi.fn(),
  subscribeLogs: vi.fn(),
  unsubscribeLogs: vi.fn(),
  reconnect: vi.fn(),
}));

vi.mock('../src/lib/feed', () => ({
  useResourceFeed: () => ({
    status: 'connected',
    subscribe: feedMocks.subscribe,
    unsubscribe: feedMocks.unsubscribe,
    subscribeLogs: feedMocks.subscribeLogs,
    unsubscribeLogs: feedMocks.unsubscribeLogs,
    reconnect: feedMocks.reconnect,
  }),
}));

const stubs = vi.hoisted((): { node: GraphNode; flux: FluxResource } => ({
  node: {
    id: 'gn-1',
    kind: 'HelmRelease',
    group: 'helm.toolkit.fluxcd.io',
    version: 'v2',
    resource: 'helmreleases',
    name: 'podinfo',
    namespace: 'apps',
    status: 'Ready',
    category: 'app',
  },
  flux: {
    kind: 'Kustomization',
    group: 'kustomize.toolkit.fluxcd.io',
    version: 'v1',
    resource: 'kustomizations',
    name: 'apps',
    namespace: 'flux-system',
    ready: 'True',
    suspended: false,
    revision: 'main@sha1:abc',
    source: '',
    message: '',
    createdAt: '2026-07-24T00:00:00Z',
  },
}));

vi.mock('../src/components/GitopsGraph', () => ({
  default: ({ onSelect }: { onSelect: (node: GraphNode) => void }) => (
    <div data-testid="gitops-graph">
      <button
        type="button"
        onClick={() => {
          onSelect(stubs.node);
        }}
      >
        select-node
      </button>
    </div>
  ),
}));

function fluxStub(testId: string) {
  return ({ onSelect }: { onSelect: (resource: FluxResource) => void }) => (
    <div data-testid={testId}>
      <button
        type="button"
        onClick={() => {
          onSelect(stubs.flux);
        }}
      >
        select-{testId}
      </button>
    </div>
  );
}

vi.mock('../src/components/FluxList', () => ({ default: fluxStub('flux-dashboard') }));
vi.mock('../src/components/FluxOverview', () => ({ default: fluxStub('flux-overview') }));
vi.mock('../src/components/FluxRoles', () => ({ default: fluxStub('flux-roles') }));

vi.mock('../src/components/InspectDrawer', () => ({
  default: ({
    target,
    containers,
    onClose,
  }: {
    target: ObjectRef | null;
    containers?: { name: string; reason?: string; restarts: number }[];
    onClose: () => void;
  }) => {
    if (target === null) {
      return <aside>Select a row to inspect it.</aside>;
    }
    return (
      <aside>
        <span data-testid="inspect-target">
          {`${target.resource}:${target.namespace}/${target.name}`}
        </span>
        <span data-testid="inspect-containers">
          {(containers ?? [])
            .map((one) => `${one.name}:${one.reason ?? ''}:${one.restarts}`)
            .join(',')}
        </span>
        <button type="button" onClick={onClose}>
          Close
        </button>
      </aside>
    );
  },
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
      return Promise.resolve({ ok: true, json: () => Promise.resolve({ categories }) });
    }),
  );
}

function resetStore(): void {
  useResourcesStore.setState({ subs: new Map() });
}

async function expandWorkloads(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await user.click(await screen.findByRole('button', { name: /Workloads/ }));
}

async function selectPod(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await expandWorkloads(user);
  await user.click(await screen.findByRole('button', { name: 'Pod' }));
}

describe('App', () => {
  beforeEach(() => {
    resetStore();
    stubFetch();
    feedMocks.subscribe.mockClear();
    feedMocks.unsubscribe.mockClear();
    feedMocks.subscribeLogs.mockClear();
    feedMocks.unsubscribeLogs.mockClear();
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
    expect(screen.getByText('Select a row to inspect it.')).toBeInTheDocument();
  });

  it('subscribes and renders rows when a resource is selected', async () => {
    useResourcesStore
      .getState()
      .applySnapshot('main#1', makeColumns(['Ready']), true, [
        makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod', cells: ['1/1'] }),
      ]);
    const user = userEvent.setup();
    render(<App />);
    await selectPod(user);
    expect(feedMocks.subscribe).toHaveBeenCalledWith('main#1', podDescriptor, '');
    expect(await screen.findByRole('button', { name: 'pod-a' })).toBeInTheDocument();
    expect(screen.getByText('1/1')).toBeInTheDocument();
  });

  it('keeps the selected row in step with the live feed', async () => {
    useResourcesStore.getState().applySnapshot('main#1', makeColumns([]), true, [
      makeRow({
        uid: 'a',
        name: 'pod-a',
        namespace: 'prod',
        containers: [{ name: 'app', state: 'running', ready: true, restarts: 0, init: false }],
      }),
    ]);
    const user = userEvent.setup();
    render(<App />);
    await selectPod(user);
    await user.click(await screen.findByRole('button', { name: 'pod-a' }));

    act(() => {
      useResourcesStore.getState().applyDelta('main#1', {
        type: 'modified',
        subId: 'main#1',
        row: makeRow({
          uid: 'a',
          name: 'pod-a',
          namespace: 'prod',
          containers: [
            {
              name: 'app',
              state: 'waiting',
              reason: 'CrashLoopBackOff',
              ready: false,
              restarts: 7,
              init: false,
            },
          ],
        }),
      });
    });

    await waitFor(() => {
      expect(screen.getByTestId('inspect-containers')).toHaveTextContent('app:CrashLoopBackOff:7');
    });
  });

  it('targets the inspector at the selected row', async () => {
    useResourcesStore
      .getState()
      .applySnapshot('main#1', makeColumns([]), true, [
        makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' }),
      ]);
    const user = userEvent.setup();
    render(<App />);
    await selectPod(user);
    await user.click(await screen.findByRole('button', { name: 'pod-a' }));

    expect(screen.getByTestId('inspect-target')).toHaveTextContent('pods:prod/pod-a');

    await user.click(screen.getByRole('button', { name: 'Close' }));

    expect(screen.getByText('Select a row to inspect it.')).toBeInTheDocument();
  });

  it('unsubscribes the previous resource when switching resources', async () => {
    const user = userEvent.setup();
    render(<App />);
    await selectPod(user);
    feedMocks.unsubscribe.mockClear();
    await user.click(screen.getByRole('button', { name: 'Deployment' }));
    expect(feedMocks.unsubscribe).toHaveBeenCalledWith('main#1');
    expect(feedMocks.subscribe).toHaveBeenCalledWith('main#2', deploymentDescriptor, '');
  });

  it('reconnects when the reconnect button is clicked', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole('button', { name: 'Reconnect' }));
    expect(feedMocks.reconnect).toHaveBeenCalledTimes(1);
  });

  it('streams logs for a selected pod through the dock', async () => {
    useResourcesStore.getState().applySnapshot('main#1', makeColumns([]), true, [
      makeRow({
        uid: 'a',
        name: 'pod-a',
        namespace: 'prod',
        containers: [{ name: 'app', state: 'running', ready: true, restarts: 0, init: false }],
      }),
    ]);
    const user = userEvent.setup();
    render(<App />);
    await selectPod(user);
    await user.click(await screen.findByRole('button', { name: 'pod-a' }));
    await user.click(screen.getByRole('button', { name: /Logs/ }));

    expect(feedMocks.subscribeLogs).toHaveBeenCalledWith(
      'logs',
      expect.objectContaining({ namespace: 'prod', name: 'pod-a', container: 'app' }),
    );
  });

  it('leaves the dock without a pod for rows that have no containers', async () => {
    useResourcesStore
      .getState()
      .applySnapshot('main#1', makeColumns([]), true, [
        makeRow({ uid: 'a', name: 'dep-a', namespace: 'prod' }),
      ]);
    const user = userEvent.setup();
    render(<App />);
    await selectPod(user);
    await user.click(await screen.findByRole('button', { name: 'dep-a' }));
    await user.click(screen.getByRole('button', { name: /Logs/ }));

    expect(screen.getByText('Select a pod to stream its logs.')).toBeInTheDocument();
    expect(feedMocks.subscribeLogs).not.toHaveBeenCalled();
  });

  it('offers regular containers before init containers in the log picker', async () => {
    useResourcesStore.getState().applySnapshot('main#1', makeColumns([]), true, [
      makeRow({
        uid: 'a',
        name: 'pod-a',
        namespace: 'prod',
        containers: [
          { name: 'copy-libs', state: 'terminated', ready: false, restarts: 0, init: true },
          { name: 'app', state: 'running', ready: true, restarts: 0, init: false },
        ],
      }),
    ]);
    const user = userEvent.setup();
    render(<App />);
    await selectPod(user);
    await user.click(await screen.findByRole('button', { name: 'pod-a' }));
    await user.click(screen.getByRole('button', { name: /Logs/ }));

    expect(feedMocks.subscribeLogs).toHaveBeenCalledWith(
      'logs',
      expect.objectContaining({ container: 'app' }),
    );
    const picker = screen.getByLabelText('Container');
    const options = within(picker)
      .getAllByRole('option')
      .map((node) => node.textContent);
    expect(options).toEqual(['app', 'copy-libs']);
  });

  it('leaves the dock without a pod for rows with an empty container list', async () => {
    useResourcesStore
      .getState()
      .applySnapshot('main#1', makeColumns([]), true, [
        makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod', containers: [] }),
      ]);
    const user = userEvent.setup();
    render(<App />);
    await selectPod(user);
    await user.click(await screen.findByRole('button', { name: 'pod-a' }));
    await user.click(screen.getByRole('button', { name: /Logs/ }));

    expect(screen.getByText('Select a pod to stream its logs.')).toBeInTheDocument();
    expect(feedMocks.subscribeLogs).not.toHaveBeenCalled();
  });

  it('targets the inspector at a selected graph node', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole('button', { name: 'Graph' }));
    expect(screen.getByTestId('gitops-graph')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'select-node' }));

    expect(screen.getByTestId('inspect-target')).toHaveTextContent('helmreleases:apps/podinfo');
  });

  it('targets the inspector from the flux table', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole('button', { name: 'Resource list' }));

    await user.click(screen.getByRole('button', { name: 'select-flux-dashboard' }));

    expect(screen.getByTestId('inspect-target')).toHaveTextContent(
      'kustomizations:flux-system/apps',
    );
  });

  it('targets the inspector from the flux overview', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole('button', { name: 'Overview' }));

    await user.click(screen.getByRole('button', { name: 'select-flux-overview' }));

    expect(screen.getByTestId('inspect-target')).toHaveTextContent(
      'kustomizations:flux-system/apps',
    );
  });

  it('targets the inspector from the by-role view', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole('button', { name: 'By role' }));

    await user.click(screen.getByRole('button', { name: 'select-flux-roles' }));

    expect(screen.getByTestId('inspect-target')).toHaveTextContent(
      'kustomizations:flux-system/apps',
    );
  });

  it('clears the target when switching views', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole('button', { name: 'Graph' }));
    await user.click(screen.getByRole('button', { name: 'select-node' }));
    expect(screen.getByTestId('inspect-target')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Resource list' }));

    expect(screen.queryByTestId('inspect-target')).not.toBeInTheDocument();
    expect(screen.getByText('Select a row to inspect it.')).toBeInTheDocument();
  });

  it('returns to the resources view when a resource is selected', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole('button', { name: 'Graph' }));
    expect(screen.getByTestId('gitops-graph')).toBeInTheDocument();
    await selectPod(user);
    expect(screen.queryByTestId('gitops-graph')).not.toBeInTheDocument();
    expect(screen.getByText('Select a row to inspect it.')).toBeInTheDocument();
  });
});

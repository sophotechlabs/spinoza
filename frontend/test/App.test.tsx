import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { Category, FluxResource, GraphNode, ObjectRef } from '../src/lib/types';

const feedMocks = vi.hoisted(
  (): {
    status: 'connecting' | 'connected' | 'disconnected';
    attempt: number;
    subscribe: ReturnType<typeof vi.fn>;
    unsubscribe: ReturnType<typeof vi.fn>;
    subscribeLogs: ReturnType<typeof vi.fn>;
    unsubscribeLogs: ReturnType<typeof vi.fn>;
    reconnect: ReturnType<typeof vi.fn>;
  } => ({
    status: 'connected',
    attempt: 0,
    subscribe: vi.fn(),
    unsubscribe: vi.fn(),
    subscribeLogs: vi.fn(),
    unsubscribeLogs: vi.fn(),
    reconnect: vi.fn(),
  }),
);

vi.mock('../src/lib/feed', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../src/lib/feed')>()),
  useResourceFeed: () => ({
    status: feedMocks.status,
    attempt: feedMocks.attempt,
    subscribe: feedMocks.subscribe,
    unsubscribe: feedMocks.unsubscribe,
    subscribeLogs: feedMocks.subscribeLogs,
    unsubscribeLogs: feedMocks.unsubscribeLogs,
    reconnect: feedMocks.reconnect,
  }),
}));

const stubs = vi.hoisted(
  (): { node: GraphNode; podNode: GraphNode; argoNode: GraphNode; flux: FluxResource } => ({
    argoNode: {
      id: 'applications/argocd/root',
      kind: 'Application',
      group: 'argoproj.io',
      version: 'v1alpha1',
      resource: 'applications',
      name: 'root',
      namespace: 'argocd',
      status: 'Synced Healthy',
      ready: 'True',
      category: 'app',
    },
    podNode: {
      id: 'gn-2',
      kind: 'Pod',
      group: '',
      version: 'v1',
      resource: 'pods',
      name: 'pod-a',
      namespace: 'prod',
      status: 'Running',
      ready: 'True',
      category: 'managed',
    },
    node: {
      id: 'gn-1',
      kind: 'HelmRelease',
      group: 'helm.toolkit.fluxcd.io',
      version: 'v2',
      resource: 'helmreleases',
      name: 'podinfo',
      namespace: 'apps',
      status: 'Ready',
      ready: 'True',
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
  }),
);

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
      <button
        type="button"
        onClick={() => {
          onSelect(stubs.podNode);
        }}
      >
        select-pod-node
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

vi.mock('../src/components/ClusterOverview', () => ({
  default: () => <div data-testid="cluster-overview" />,
}));
vi.mock('../src/components/HelmReleases', () => ({
  default: ({ onSelectResource }: { onSelectResource: (ref: ObjectRef) => void }) => (
    <div data-testid="helm-releases">
      <button
        type="button"
        onClick={() => {
          onSelectResource({
            group: '',
            version: 'v1',
            resource: 'configmaps',
            namespace: 'demo',
            name: 'live-check',
          });
        }}
      >
        select-helm-resource
      </button>
    </div>
  ),
}));
vi.mock('../src/components/FluxList', () => ({ default: fluxStub('flux-dashboard') }));
vi.mock('../src/components/FluxRoles', () => ({ default: fluxStub('flux-roles') }));
vi.mock('../src/components/ArgoApps', () => ({
  default: ({ onSelect }: { onSelect: (ref: unknown) => void }) => (
    <button
      type="button"
      data-testid="argo-apps"
      onClick={() => {
        onSelect({
          group: 'argoproj.io',
          version: 'v1alpha1',
          resource: 'applications',
          namespace: 'argocd',
          name: 'root',
        });
      }}
    >
      select-argo
    </button>
  ),
}));

vi.mock('../src/components/ArgoGraph', () => ({
  default: ({ onSelect }: { onSelect: (node: GraphNode) => void }) => (
    <button
      type="button"
      data-testid="argo-graph"
      onClick={() => {
        onSelect(stubs.argoNode);
      }}
    >
      select-argo-node
    </button>
  ),
}));

vi.mock('../src/components/ArgoList', () => ({
  default: ({ onSelect }: { onSelect: (ref: ObjectRef) => void }) => (
    <button
      type="button"
      data-testid="argo-list"
      onClick={() => {
        onSelect({
          group: 'argoproj.io',
          version: 'v1alpha1',
          resource: 'appprojects',
          namespace: 'argocd',
          name: 'default',
        });
      }}
    >
      select-argo-project
    </button>
  ),
}));

vi.mock('../src/components/ForwardsPanel', () => ({
  default: () => <div data-testid="forwards-panel" />,
}));

vi.mock('../src/components/TerminalPanel', () => ({
  default: ({ target }: { target: { pod: string; container: string } }) => (
    <div data-testid="terminal-panel">
      {target.pod}/{target.container}
    </div>
  ),
}));

vi.mock('../src/components/ContextPicker', () => ({
  default: ({ onSwitched }: { onSwitched: () => void }) => (
    <button type="button" data-testid="context-changed" onClick={onSwitched}>
      switch context
    </button>
  ),
}));

vi.mock('../src/components/ViewSwitch', () => ({
  default: ({ onLeft }: { onLeft: () => void }) => (
    <button type="button" data-testid="left-for-desktop" onClick={onLeft}>
      desktop
    </button>
  ),
}));

vi.mock('../src/components/PanelChrome', () => ({
  default: ({
    target,
    onClose,
    children,
  }: {
    target: { resource: string; namespace: string; name: string } | null;
    onClose: () => void;
    children: React.ReactNode;
  }) => (
    <div>
      {target !== null && (
        <span data-testid="inspect-target">
          {`${target.resource}:${target.namespace}/${target.name}`}
        </span>
      )}
      <button type="button" onClick={onClose}>
        Close
      </button>
      {children}
    </div>
  ),
}));

vi.mock('../src/components/InspectOverview', () => ({
  default: ({
    containers,
  }: {
    containers?: { name: string; reason?: string; restarts: number }[];
  }) => (
    <span data-testid="inspect-containers">
      {(containers ?? []).map((one) => `${one.name}:${one.reason ?? ''}:${one.restarts}`).join(',')}
    </span>
  ),
}));

vi.mock('../src/components/InspectLogs', () => ({
  default: ({ containers }: { containers: string[] }) => (
    <span data-testid="inspect-log-feed">{containers.join(',')}</span>
  ),
}));

import App from '../src/App';
import { useResourcesStore } from '../src/store/resources';
import { clearRecents, rememberObject } from '../src/store/recents';
import { DEFAULT_NAMESPACE, useNamespaceStore } from '../src/store/namespace';
import { notifyOk, useToastsStore } from '../src/store/toasts';
import { setUnsaved } from '../src/lib/unsaved';
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

const argoDescriptor = makeDescriptor({
  group: 'argoproj.io',
  version: 'v1alpha1',
  resource: 'applications',
  kind: 'Application',
});

const kustomizationDescriptor = makeDescriptor({
  group: 'kustomize.toolkit.fluxcd.io',
  version: 'v1',
  resource: 'kustomizations',
  kind: 'Kustomization',
});

const categories: Category[] = [
  makeCategory('Workloads', [podDescriptor, deploymentDescriptor]),
  makeCategory('Custom resources', [kustomizationDescriptor, argoDescriptor]),
];

const clusterHit = {
  group: 'apps',
  version: 'v1',
  resource: 'deployments',
  kind: 'Deployment',
  namespace: 'airbyte',
  name: 'airbyte-server',
};

const objectDetail = {
  apiVersion: 'v1',
  kind: 'Pod',
  name: 'pod-a',
  namespace: 'prod',
  uid: 'uid-pod-a',
  createdAt: '2026-08-03T09:00:00Z',
  containers: ['app'],
  yaml: 'kind: Pod\n',
};

function detailFor(url: string) {
  if (url.includes('resource=pods')) {
    return objectDetail;
  }
  if (url.includes('resource=helmreleases')) {
    return {
      ...objectDetail,
      apiVersion: 'helm.toolkit.fluxcd.io/v2',
      kind: 'HelmRelease',
      name: 'podinfo',
      namespace: 'apps',
      containers: undefined,
    };
  }
  return {
    ...objectDetail,
    apiVersion: 'apps/v1',
    kind: 'Deployment',
    name: 'dep-a',
    containers: undefined,
  };
}

function stubFetch(): void {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string) => {
      if (url.startsWith('/api/contexts')) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              current: { kubeconfig: '', name: 'kind-dev' },
              kubeconfigs: [
                {
                  label: '/home/arch/.kube/config',
                  path: '',
                  removable: false,
                  contexts: [{ name: 'kind-dev', cluster: 'kind-dev' }],
                },
              ],
            }),
        });
      }
      if (url === '/api/metrics') {
        return Promise.resolve({ ok: true, json: () => Promise.resolve({ pods: {}, nodes: {} }) });
      }
      if (url.startsWith('/api/object')) {
        return Promise.resolve({ ok: true, json: () => Promise.resolve(detailFor(url)) });
      }
      if (url.startsWith('/api/events')) {
        return Promise.resolve({ ok: true, json: () => Promise.resolve([]) });
      }
      if (url.startsWith('/api/portforward')) {
        return Promise.resolve({ ok: true, json: () => Promise.resolve([]) });
      }
      if (url.startsWith('/api/namespaces')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ names: ['airbyte', 'default', 'prod'] }),
        });
      }
      if (url.startsWith('/api/search')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ hits: [clusterHit], truncated: false }),
        });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve({ categories }) });
    }),
  );
}

function resetStore(): void {
  useResourcesStore.setState({ subs: new Map() });
}

function openAt(hash: string): void {
  window.history.replaceState(null, '', hash);
}

function goBackTo(hash: string): void {
  act(() => {
    window.history.replaceState(null, '', hash);
    window.dispatchEvent(new PopStateEvent('popstate'));
  });
}

async function expandWorkloads(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await user.click(await screen.findByRole('button', { name: /Workloads/ }));
}

async function selectPod(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await expandWorkloads(user);
  await user.click(await screen.findByRole('button', { name: 'Pod' }));
}

async function selectDeployment(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await expandWorkloads(user);
  await user.click(await screen.findByRole('button', { name: 'Deployment' }));
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
    expect(screen.getByRole('status', { name: 'The cluster feed is connected' })).toBeVisible();
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
    expect(feedMocks.subscribe).toHaveBeenCalledWith(
      'main#1',
      expect.objectContaining({ group: '', version: 'v1', resource: 'pods', kind: 'Pod' }),
      'default',
    );
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
      useResourcesStore.getState().applyDeltas('main#1', [
        {
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
        },
      ]);
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
    expect(feedMocks.subscribe).toHaveBeenCalledWith(
      'main#2',
      expect.objectContaining({ group: 'apps', version: 'v1', resource: 'deployments' }),
      'default',
    );
  });

  it('drops the selection when the cluster changes', async () => {
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

    await user.click(screen.getByTestId('context-changed'));

    expect(screen.getByText('Select a row to inspect it.')).toBeInTheDocument();
    expect(screen.queryByTestId('inspect-target')).not.toBeInTheDocument();
  });

  it('starts the notification history over when the cluster changes', async () => {
    const user = userEvent.setup();
    render(<App />);
    notifyOk('Deleted Pod web-0');
    expect(useToastsStore.getState().history).toHaveLength(1);

    await user.click(screen.getByTestId('context-changed'));

    expect(useToastsStore.getState().history).toHaveLength(0);
  });

  it('unsubscribes the old cluster resource when the cluster changes', async () => {
    const user = userEvent.setup();
    render(<App />);
    await selectPod(user);
    feedMocks.unsubscribe.mockClear();

    await user.click(screen.getByTestId('context-changed'));

    expect(feedMocks.unsubscribe).toHaveBeenCalledWith('main#1');
    expect(feedMocks.reconnect).toHaveBeenCalled();
  });

  it('reconnects when the reconnect button is clicked', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole('button', { name: 'Reconnect' }));
    expect(feedMocks.reconnect).toHaveBeenCalledTimes(1);
  });

  it('hands the log feed to the inspector, which is the only place logs stream', async () => {
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

    await user.click(screen.getByRole('tab', { name: 'Logs' }));

    expect(screen.getByTestId('inspect-log-feed')).toHaveTextContent('app');
  });

  it('offers no shell for an object that is not a pod', async () => {
    useResourcesStore
      .getState()
      .applySnapshot('main#1', makeColumns([]), true, [
        makeRow({ uid: 'a', name: 'dep-a', namespace: 'prod' }),
      ]);
    const user = userEvent.setup();
    render(<App />);
    await selectDeployment(user);
    await user.click(await screen.findByRole('button', { name: 'dep-a' }));

    await user.click(screen.getByRole('tab', { name: 'Terminal' }));

    await waitFor(() => {
      expect(screen.getByText(/No shells open/)).toBeInTheDocument();
    });
    expect(screen.queryByRole('button', { name: /Shell in/ })).not.toBeInTheDocument();
  });

  it('offers regular containers before init containers in the picker', async () => {
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
    await user.click(screen.getByRole('tab', { name: 'Terminal' }));

    const picker = screen.getByLabelText('Container');
    const options = within(picker)
      .getAllByRole('option')
      .map((node) => node.textContent);
    expect(options).toEqual(['app', 'copy-libs']);
  });

  it('opens the terminal for a pod whose row carries no containers', async () => {
    useResourcesStore
      .getState()
      .applySnapshot('main#1', makeColumns([]), true, [
        makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod', containers: [] }),
      ]);
    const user = userEvent.setup();
    render(<App />);
    await selectPod(user);
    await user.click(await screen.findByRole('button', { name: 'pod-a' }));

    await waitFor(() => {
      expect(screen.getByRole('tab', { name: 'Terminal' })).toHaveAttribute(
        'aria-disabled',
        'false',
      );
    });
  });

  it('targets the inspector at a selected graph node', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(await screen.findByRole('button', { name: 'Flux Graph' }));
    expect(screen.getByTestId('gitops-graph')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'select-node' }));

    expect(screen.getByTestId('inspect-target')).toHaveTextContent('helmreleases:apps/podinfo');
  });

  it('targets the inspector from the flux table', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(await screen.findByRole('button', { name: 'Flux Resource list' }));

    await user.click(screen.getByRole('button', { name: 'select-flux-dashboard' }));

    expect(screen.getByTestId('inspect-target')).toHaveTextContent(
      'kustomizations:flux-system/apps',
    );
  });

  it('targets the inspector from the by-role view', async () => {
    const user = userEvent.setup();
    render(<App />);
    const gitops = await screen.findByLabelText('Flux views');
    await user.click(within(gitops).getByRole('button', { name: 'Flux Overview' }));

    await user.click(screen.getByRole('button', { name: 'select-flux-roles' }));

    expect(screen.getByTestId('inspect-target')).toHaveTextContent(
      'kustomizations:flux-system/apps',
    );
  });

  it('clears the target when switching views', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(await screen.findByRole('button', { name: 'Flux Graph' }));
    await user.click(screen.getByRole('button', { name: 'select-node' }));
    expect(screen.getByTestId('inspect-target')).toBeInTheDocument();

    await user.click(await screen.findByRole('button', { name: 'Flux Resource list' }));

    expect(screen.queryByTestId('inspect-target')).not.toBeInTheDocument();
    expect(screen.getByText('Select a row to inspect it.')).toBeInTheDocument();
  });

  it('opens the cluster overview from the sidebar', async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole('button', { name: 'Cluster' }));

    expect(screen.getByTestId('cluster-overview')).toBeInTheDocument();
    expect(window.location.hash).toContain('view=cluster');
  });

  it('opens the helm releases from the sidebar', async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole('button', { name: 'Helm releases' }));

    expect(screen.getByTestId('helm-releases')).toBeInTheDocument();
    expect(window.location.hash).toContain('view=helm');
  });

  it('returns to the resources view when a resource is selected', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(await screen.findByRole('button', { name: 'Flux Graph' }));
    expect(screen.getByTestId('gitops-graph')).toBeInTheDocument();
    await selectPod(user);
    expect(screen.queryByTestId('gitops-graph')).not.toBeInTheDocument();
    expect(screen.getByText('Select a row to inspect it.')).toBeInTheDocument();
  });
});

describe('the address bar', () => {
  beforeEach(() => {
    resetStore();
    stubFetch();
    feedMocks.subscribe.mockClear();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    resetStore();
  });

  it('records the chosen resource so the page can be linked', async () => {
    const user = userEvent.setup();
    render(<App />);

    await selectPod(user);

    expect(window.location.hash).toContain('resource=pods');
    expect(window.location.hash).toContain('kind=Pod');
  });

  it('records the selected object next to its resource', async () => {
    useResourcesStore
      .getState()
      .applySnapshot('main#1', makeColumns([]), true, [
        makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' }),
      ]);
    const user = userEvent.setup();
    render(<App />);
    await selectPod(user);

    await user.click(await screen.findByRole('button', { name: 'pod-a' }));

    expect(window.location.hash).toContain('namespace=prod');
    expect(window.location.hash).toContain('name=pod-a');
    expect(window.location.hash).not.toContain('selResource');
  });

  it('spells out an object picked from a view with no table behind it', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(await screen.findByRole('button', { name: 'Flux Graph' }));

    await user.click(screen.getByRole('button', { name: 'select-node' }));

    expect(window.location.hash).toContain('selResource=helmreleases');
    expect(window.location.hash).toContain('view=gitops');
  });

  it('subscribes and selects again from a link that was opened cold', async () => {
    openAt('#version=v1&resource=pods&kind=Pod&namespace=prod&name=pod-a');
    useResourcesStore
      .getState()
      .applySnapshot('main#0', makeColumns([]), true, [
        makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' }),
      ]);

    render(<App />);

    expect(feedMocks.subscribe).toHaveBeenCalledWith(
      'main#0',
      expect.objectContaining({ resource: 'pods' }),
      'default',
    );
    expect(await screen.findByTestId('inspect-target')).toHaveTextContent('pods:prod/pod-a');
  });

  it('keeps the resource but drops an object from another cluster', async () => {
    openAt('#context=other-cluster&version=v1&resource=pods&kind=Pod&namespace=prod&name=pod-a');
    useResourcesStore
      .getState()
      .applySnapshot('main#0', makeColumns([]), true, [
        makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' }),
      ]);

    render(<App />);

    await waitFor(() => {
      expect(window.location.hash).toContain('context=kind-dev');
    });
    expect(screen.queryByTestId('inspect-target')).not.toBeInTheDocument();
    expect(await screen.findByRole('button', { name: 'pod-a' })).toBeInTheDocument();
  });

  it('follows the back button out of a selection', async () => {
    useResourcesStore
      .getState()
      .applySnapshot('main#1', makeColumns([]), true, [
        makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' }),
      ]);
    const user = userEvent.setup();
    render(<App />);
    await selectPod(user);
    await user.click(await screen.findByRole('button', { name: 'pod-a' }));
    expect(screen.getByTestId('inspect-target')).toBeInTheDocument();

    goBackTo('#version=v1&resource=pods&kind=Pod');

    expect(screen.queryByTestId('inspect-target')).not.toBeInTheDocument();
  });

  it('names the object, the resource and the cluster in the tab title', async () => {
    useResourcesStore
      .getState()
      .applySnapshot('main#1', makeColumns([]), true, [
        makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' }),
      ]);
    const user = userEvent.setup();
    render(<App />);
    await selectPod(user);
    await user.click(await screen.findByRole('button', { name: 'pod-a' }));

    await waitFor(() => {
      expect(document.title).toBe('pod-a pods kind-dev - Spinoza');
    });
  });

  it('leaves the cluster out of the title when it cannot be read', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((url: string) => {
        if (url.startsWith('/api/contexts')) {
          return Promise.reject(new Error('offline'));
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({ categories }) });
      }),
    );
    const user = userEvent.setup();
    render(<App />);

    await selectPod(user);

    await waitFor(() => {
      expect(document.title).toBe('pods - Spinoza');
    });
    expect(window.location.hash).not.toContain('context=');
  });

  it('forgets the resource when the cluster changes', async () => {
    const user = userEvent.setup();
    render(<App />);
    await selectPod(user);
    expect(window.location.hash).toContain('resource=pods');

    await user.click(screen.getByTestId('context-changed'));

    expect(window.location.hash).not.toContain('resource=pods');
  });
});

describe('a selection that outlives its row', () => {
  beforeEach(() => {
    resetStore();
    stubFetch();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    resetStore();
  });

  it('re-binds to the same pod after it is recreated under a new uid', async () => {
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
    expect(screen.getByTestId('inspect-containers')).toHaveTextContent('app::0');

    act(() => {
      useResourcesStore
        .getState()
        .applyDeltas('main#1', [{ type: 'deleted', subId: 'main#1', uid: 'a' }]);
      useResourcesStore.getState().applyDeltas('main#1', [
        {
          type: 'added',
          subId: 'main#1',
          row: makeRow({
            uid: 'b',
            name: 'pod-a',
            namespace: 'prod',
            containers: [{ name: 'app', state: 'running', ready: true, restarts: 3, init: false }],
          }),
        },
      ]);
    });

    expect(screen.getByTestId('inspect-containers')).toHaveTextContent('app::3');
    expect(screen.getByTestId('inspect-target')).toHaveTextContent('pods:prod/pod-a');
  });

  it('opens the terminal for a pod picked out of the graph', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(await screen.findByRole('button', { name: 'Flux Graph' }));

    await user.click(screen.getByRole('button', { name: 'select-pod-node' }));

    await waitFor(() => {
      expect(screen.getByRole('tab', { name: 'Terminal' })).toHaveAttribute(
        'aria-disabled',
        'false',
      );
    });
    expect(screen.getByTestId('inspect-target')).toHaveTextContent('pods:prod/pod-a');
  });

  it('leaves a selection from another resource unresolved', async () => {
    useResourcesStore
      .getState()
      .applySnapshot('main#1', makeColumns([]), true, [
        makeRow({ uid: 'a', name: 'podinfo', namespace: 'apps' }),
      ]);
    const user = userEvent.setup();
    render(<App />);
    await selectPod(user);
    await user.click(await screen.findByRole('button', { name: 'Flux Graph' }));

    await user.click(screen.getByRole('button', { name: 'select-node' }));

    expect(screen.getByTestId('inspect-target')).toHaveTextContent('helmreleases:apps/podinfo');
    expect(screen.getByRole('tab', { name: 'Terminal' })).toHaveAttribute('aria-disabled', 'false');
  });
});

describe('the command palette and shortcuts', () => {
  beforeEach(() => {
    resetStore();
    stubFetch();
    clearRecents();
    HTMLDialogElement.prototype.showModal = function showModal(this: HTMLDialogElement) {
      this.open = true;
    };
    HTMLDialogElement.prototype.close = function close(this: HTMLDialogElement) {
      this.open = false;
    };
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    clearRecents();
    resetStore();
    useNamespaceStore.getState().choose(DEFAULT_NAMESPACE);
  });

  function press(key: string, init: KeyboardEventInit = {}): void {
    act(() => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, ...init }));
    });
  }

  it('opens on Ctrl K and jumps to the chosen kind', async () => {
    const user = userEvent.setup();
    render(<App />);

    press('k', { ctrlKey: true });

    await user.click(await screen.findByRole('button', { name: /Deployment/ }));

    expect(await screen.findByLabelText('Filter by name')).toBeInTheDocument();
    expect(feedMocks.subscribe).toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({ resource: 'deployments' }),
      'default',
    );
  });

  it('opens the list, namespace and filter behind an object found in the cluster', async () => {
    const user = userEvent.setup();
    render(<App />);

    press('k', { ctrlKey: true });
    await user.type(await screen.findByLabelText(/Search resources/), 'airbyte');
    await user.click(await screen.findByRole('button', { name: /airbyte\/airbyte-server/ }));

    expect(await screen.findByLabelText('Filter by name')).toHaveValue('airbyte');
    expect(screen.getByLabelText('Namespace')).toHaveValue('airbyte');
    expect(feedMocks.subscribe).toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({ resource: 'deployments' }),
      'airbyte',
    );
    expect(screen.getByTestId('inspect-target')).toHaveTextContent(
      'deployments:airbyte/airbyte-server',
    );
  });

  it('only inspects an object whose kind discovery no longer knows', async () => {
    const user = userEvent.setup();
    rememberObject({
      group: 'old.example.com',
      version: 'v1',
      resource: 'widgets',
      namespace: 'prod',
      name: 'w-1',
    });
    render(<App />);

    press('k', { ctrlKey: true });
    await user.click(await screen.findByRole('button', { name: /prod\/w-1/ }));

    expect(await screen.findByTestId('inspect-target')).toHaveTextContent('widgets:prod/w-1');
    expect(screen.getByText('Select a resource to view.')).toBeInTheDocument();
    expect(screen.getByLabelText('Namespace')).toHaveValue('default');
  });

  it('switches view from the palette', async () => {
    const user = userEvent.setup();
    render(<App />);

    press('k', { ctrlKey: true });
    await user.click(await screen.findByRole('button', { name: /Flux resources/ }));

    expect(await screen.findByTestId('flux-dashboard')).toBeInTheDocument();
  });

  it('opens the search button in the top bar', async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole('button', { name: /Search/ }));

    expect(await screen.findByLabelText(/Search resources/)).toBeInTheDocument();
  });

  it('lists a visited object as recent and reopens it', async () => {
    const user = userEvent.setup();
    useResourcesStore
      .getState()
      .applySnapshot('main#1', makeColumns([]), true, [
        makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' }),
      ]);
    render(<App />);
    await selectPod(user);
    await user.click(await screen.findByRole('button', { name: 'pod-a' }));
    await user.click(screen.getByRole('button', { name: 'Close' }));

    press('k', { ctrlKey: true });

    expect(await screen.findByRole('button', { name: /prod\/pod-a/ })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /prod\/pod-a/ }));
    expect(await screen.findByTestId('inspect-target')).toHaveTextContent('pods:prod/pod-a');
  });

  it('shows the shortcut list on ?', async () => {
    render(<App />);

    press('?');

    expect(await screen.findByText('Open the command palette')).toBeInTheDocument();
  });

  it('opens settings from the gear at the appearance section', async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole('button', { name: 'Settings' }));

    expect(await screen.findByLabelText('Theme preference')).toBeInTheDocument();
  });

  it('closes settings again from its own Close button', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole('button', { name: 'Settings' }));
    await screen.findByLabelText('Theme preference');

    const dialog = screen.getByRole('dialog', { name: 'Settings' });
    expect(dialog).toHaveAttribute('open');

    await user.click(screen.getByRole('button', { name: 'Close' }));

    await waitFor(() => {
      expect(dialog).not.toHaveAttribute('open');
    });
  });

  it('jumps to the filter on /', async () => {
    const user = userEvent.setup();
    render(<App />);
    await selectPod(user);
    const filter = await screen.findByLabelText('Filter by name');

    press('/');

    expect(document.activeElement).toBe(filter);
  });

  it('closes the palette on Escape before touching the selection', async () => {
    const user = userEvent.setup();
    useResourcesStore
      .getState()
      .applySnapshot('main#1', makeColumns([]), true, [
        makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' }),
      ]);
    render(<App />);
    await selectPod(user);
    await user.click(await screen.findByRole('button', { name: 'pod-a' }));
    press('k', { ctrlKey: true });
    await screen.findByLabelText(/Search resources/);

    press('Escape');

    expect(screen.getByTestId('inspect-target')).toBeInTheDocument();
  });

  it('closes the shortcut list on Escape before touching the selection', async () => {
    const user = userEvent.setup();
    useResourcesStore
      .getState()
      .applySnapshot('main#1', makeColumns([]), true, [
        makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' }),
      ]);
    render(<App />);
    await selectPod(user);
    await user.click(await screen.findByRole('button', { name: 'pod-a' }));
    press('?');
    await screen.findByText('Open the command palette');

    press('Escape');

    expect(screen.getByTestId('inspect-target')).toBeInTheDocument();
  });

  it('clears the selection on Escape once nothing is open', async () => {
    const user = userEvent.setup();
    useResourcesStore
      .getState()
      .applySnapshot('main#1', makeColumns([]), true, [
        makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' }),
      ]);
    render(<App />);
    await selectPod(user);
    await user.click(await screen.findByRole('button', { name: 'pod-a' }));

    press('Escape');

    await waitFor(() => {
      expect(screen.queryByTestId('inspect-target')).not.toBeInTheDocument();
    });
  });

  it('does nothing on Escape with nothing open and nothing selected', () => {
    render(<App />);

    press('Escape');

    expect(screen.getByRole('status', { name: 'The cluster feed is connected' })).toBeVisible();
  });

  it('forgets recent objects when the cluster changes', async () => {
    const user = userEvent.setup();
    useResourcesStore
      .getState()
      .applySnapshot('main#1', makeColumns([]), true, [
        makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' }),
      ]);
    render(<App />);
    await selectPod(user);
    await user.click(await screen.findByRole('button', { name: 'pod-a' }));

    await user.click(screen.getByTestId('context-changed'));
    press('k', { ctrlKey: true });

    await screen.findByLabelText(/Search resources/);
    expect(screen.queryByRole('button', { name: /prod\/pod-a/ })).not.toBeInTheDocument();
  });
});

describe('a feed that dropped', () => {
  beforeEach(() => {
    resetStore();
    stubFetch();
    useToastsStore.getState().clear();
    feedMocks.status = 'connected';
    feedMocks.attempt = 0;
    feedMocks.reconnect.mockClear();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    feedMocks.status = 'connected';
    feedMocks.attempt = 0;
    useToastsStore.getState().clear();
    resetStore();
  });

  it('says so across the content area and counts the retries', () => {
    feedMocks.status = 'disconnected';
    feedMocks.attempt = 2;
    render(<App />);

    const banner = screen.getByRole('status', { name: 'The cluster feed dropped' });
    expect(banner).toHaveTextContent('The live connection dropped');
    expect(banner).toHaveTextContent('attempt 2');
  });

  it('marks the content area as no longer live', () => {
    feedMocks.status = 'disconnected';
    render(<App />);

    expect(document.querySelector('[aria-busy="true"]')).not.toBeNull();
  });

  it('leaves the content area alone while connected', () => {
    render(<App />);

    expect(document.querySelector('[aria-busy="true"]')).toBeNull();
    expect(screen.queryByText(/The live connection dropped/)).not.toBeInTheDocument();
  });

  it('reconnects on demand from the banner', async () => {
    const user = userEvent.setup();
    feedMocks.status = 'disconnected';
    render(<App />);

    await user.click(screen.getByRole('button', { name: 'Reconnect now' }));

    expect(feedMocks.reconnect).toHaveBeenCalledTimes(1);
  });

  it('says out loud when the connection comes back', () => {
    feedMocks.status = 'disconnected';
    const view = render(<App />);
    expect(useToastsStore.getState().toasts).toHaveLength(0);

    feedMocks.status = 'connected';
    view.rerender(<App />);

    expect(useToastsStore.getState().toasts).toEqual([
      expect.objectContaining({ tone: 'ok', message: 'Reconnected to the cluster' }),
    ]);
  });

  it('does not congratulate itself on the first connect', () => {
    render(<App />);

    expect(useToastsStore.getState().toasts).toHaveLength(0);
  });

  it('stays quiet while it is still reconnecting', () => {
    feedMocks.status = 'disconnected';
    const view = render(<App />);

    feedMocks.status = 'connecting';
    feedMocks.attempt = 1;
    view.rerender(<App />);

    expect(useToastsStore.getState().toasts).toHaveLength(0);
    expect(screen.getByRole('status', { name: 'The cluster feed dropped' })).toHaveTextContent(
      'attempt 1',
    );
  });
});

describe('navigating away from an unsaved draft', () => {
  beforeEach(() => {
    resetStore();
    stubFetch();
    setUnsaved(false);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    setUnsaved(false);
    resetStore();
  });

  async function selectPodA(user: ReturnType<typeof userEvent.setup>): Promise<void> {
    useResourcesStore
      .getState()
      .applySnapshot('main#1', makeColumns([]), true, [
        makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' }),
      ]);
    await selectPod(user);
    await user.click(await screen.findByRole('button', { name: 'pod-a' }));
  }

  it('asks before dropping the draft and stays put when you say no', async () => {
    const user = userEvent.setup();
    render(<App />);
    await selectPodA(user);
    setUnsaved(true);
    vi.stubGlobal('confirm', vi.fn().mockReturnValue(false));

    await user.click(screen.getByRole('button', { name: 'Close' }));

    expect(screen.getByTestId('inspect-target')).toBeInTheDocument();
  });

  it('lets go once you agree', async () => {
    const user = userEvent.setup();
    render(<App />);
    await selectPodA(user);
    setUnsaved(true);
    vi.stubGlobal('confirm', vi.fn().mockReturnValue(true));

    await user.click(screen.getByRole('button', { name: 'Close' }));

    await waitFor(() => {
      expect(screen.queryByTestId('inspect-target')).not.toBeInTheDocument();
    });
  });

  it('guards a jump to another resource', async () => {
    const user = userEvent.setup();
    render(<App />);
    await selectPodA(user);
    setUnsaved(true);
    vi.stubGlobal('confirm', vi.fn().mockReturnValue(false));

    await user.click(await screen.findByRole('button', { name: 'Deployment' }));

    expect(screen.getByTestId('inspect-target')).toBeInTheDocument();
  });

  it('guards a jump to another view', async () => {
    const user = userEvent.setup();
    render(<App />);
    await selectPodA(user);
    setUnsaved(true);
    vi.stubGlobal('confirm', vi.fn().mockReturnValue(false));

    await user.click(await screen.findByRole('button', { name: 'Flux Graph' }));

    expect(screen.getByTestId('inspect-target')).toBeInTheDocument();
  });

  it('guards a jump to an object found in the cluster', async () => {
    const user = userEvent.setup();
    HTMLDialogElement.prototype.showModal = function showModal(this: HTMLDialogElement) {
      this.open = true;
    };
    HTMLDialogElement.prototype.close = function close(this: HTMLDialogElement) {
      this.open = false;
    };
    render(<App />);
    await selectPodA(user);
    setUnsaved(true);
    vi.stubGlobal('confirm', vi.fn().mockReturnValue(false));
    await user.click(screen.getByRole('button', { name: /Search/ }));
    await user.type(await screen.findByLabelText(/Search resources/), 'airbyte');

    await user.click(await screen.findByRole('button', { name: /airbyte\/airbyte-server/ }));

    expect(screen.getByTestId('inspect-target')).toHaveTextContent('pods:prod/pod-a');
    expect(within(screen.getByRole('banner')).getByLabelText('Namespace')).toHaveValue('default');
  });

  it('guards a jump to another object', async () => {
    const user = userEvent.setup();
    render(<App />);
    await selectPodA(user);
    useResourcesStore
      .getState()
      .applySnapshot('main#1', makeColumns([]), true, [
        makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' }),
        makeRow({ uid: 'b', name: 'pod-b', namespace: 'prod' }),
      ]);
    setUnsaved(true);
    vi.stubGlobal('confirm', vi.fn().mockReturnValue(false));

    await user.click(await screen.findByRole('button', { name: 'pod-b' }));

    expect(screen.getByTestId('inspect-target')).toHaveTextContent('pods:prod/pod-a');
  });
});

describe('finding your way in by keyboard', () => {
  beforeEach(() => {
    resetStore();
    stubFetch();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    resetStore();
  });

  it('names the app with a heading a screen reader can find', () => {
    render(<App />);

    expect(screen.getByRole('heading', { level: 1, name: 'Spinoza' })).toBeInTheDocument();
  });

  it('puts the content in a main landmark', () => {
    render(<App />);

    expect(screen.getByRole('main')).toBeInTheDocument();
  });

  it('offers a skip link that points at that landmark', () => {
    render(<App />);

    const skip = screen.getByRole('link', { name: 'Skip to the content' });
    expect(skip).toHaveAttribute('href', '#content');
    expect(screen.getByRole('main')).toHaveAttribute('id', 'content');
  });

  it('makes the landmark focusable so the skip link lands somewhere', () => {
    render(<App />);

    expect(screen.getByRole('main')).toHaveAttribute('tabindex', '-1');
  });

  it('keeps the skip link out of the way until it is focused', () => {
    render(<App />);

    expect(screen.getByRole('link', { name: 'Skip to the content' }).className).toContain(
      'sr-only',
    );
  });

  it('tells the tab it can be closed once the window has spinoza back', async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByTestId('left-for-desktop'));

    expect(screen.getByText('Spinoza is back in its window')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Stay here' }));

    expect(screen.queryByText('Spinoza is back in its window')).not.toBeInTheDocument();
  });

  it('opens the argo graph and targets the inspector from it', async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(await screen.findByRole('button', { name: 'Argo CD Graph' }));
    expect(await screen.findByTestId('argo-graph')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'select-argo-node' }));

    expect(screen.getByTestId('inspect-target')).toHaveTextContent('applications:argocd/root');
  });

  it('opens the argo resource list and targets the inspector from it', async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(await screen.findByRole('button', { name: 'Argo CD Resource list' }));
    expect(await screen.findByTestId('argo-list')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'select-argo-project' }));

    expect(screen.getByTestId('inspect-target')).toHaveTextContent('appprojects:argocd/default');
  });

  it('opens the argo overview and targets the inspector from it', async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(await screen.findByRole('button', { name: 'Argo CD Overview' }));
    expect(await screen.findByTestId('argo-apps')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'select-argo' }));

    expect(screen.getByTestId('inspect-target')).toHaveTextContent('applications:argocd/root');
  });
});

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { GitopsApp, GraphNode, ObjectRef, GitopsIssue } from '../../src/lib/types';

const fitViewSpy = vi.fn();

vi.mock('@xyflow/react', () => ({
  useReactFlow: () => ({ fitView: fitViewSpy }),
  ReactFlow: ({
    nodes,
    onNodeClick,
  }: {
    nodes: { id: string; data: { label: string; node: GraphNode } }[];
    onNodeClick: (event: unknown, node: { data: { node: GraphNode } }) => void;
  }) => (
    <div data-testid="react-flow">
      {nodes.map((node) => (
        <button
          key={node.id}
          type="button"
          onClick={() => {
            onNodeClick(null, node);
          }}
        >
          {node.data.label}
        </button>
      ))}
    </div>
  ),
  Background: () => <div />,
  Controls: () => <div />,
}));

import GitopsAppPanel from '../../src/components/GitopsAppPanel';
import { useContextsStore } from '../../src/store/contexts';

const target: ObjectRef = {
  group: 'argoproj.io',
  version: 'v1alpha1',
  resource: 'applications',
  namespace: 'argocd',
  name: 'podinfo',
};

const showModal = vi.fn(function showModal(this: HTMLDialogElement) {
  this.open = true;
});

const close = vi.fn(function close(this: HTMLDialogElement) {
  this.open = false;
});

function appWith(extra: Partial<GitopsApp> = {}): GitopsApp {
  return {
    ref: target,
    controller: 'argocd',
    kind: 'Application',
    name: 'podinfo',
    namespace: 'argocd',
    source: {
      repo: 'https://example.test/apps',
      path: 'podinfo',
      target: 'main',
      destination: 'web',
      project: 'default',
      syncMode: 'auto',
      policy: 'prune',
    },
    state: { sync: 'OutOfSync', health: 'Healthy', revision: 'abc1234' },
    issues: [],
    resources: [],
    history: [],
    ...extra,
  };
}

const deployment = {
  group: 'apps',
  version: 'v1',
  resource: 'deployments',
  kind: 'Deployment',
  name: 'web',
  namespace: 'shop',
  sync: 'OutOfSync',
  health: 'Healthy',
  drift: [{ path: 'spec.replicas', declared: '1', live: '3' }],
  events: [
    {
      type: 'Normal' as const,
      reason: 'Scaled',
      message: 'scaled up',
      count: 1,
      source: 'x',
      firstSeen: '',
      lastSeen: '',
    },
  ],
};

function serve(app: GitopsApp, graph: unknown = { nodes: [], edges: [] }) {
  const fetchMock = vi.fn((url: string) => {
    if (url.includes('/api/gitops/app/graph')) {
      return Promise.resolve({ ok: true, json: () => Promise.resolve(graph) });
    }
    if (url.includes('/api/gitops/app')) {
      return Promise.resolve({ ok: true, json: () => Promise.resolve(app) });
    }
    return Promise.resolve({ ok: true, json: () => Promise.resolve({ action: 'sync' }) });
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

function openCluster() {
  useContextsStore.getState().setList({
    current: { kubeconfig: '', name: 'p-mk1' },
    kubeconfigs: [],
    protection: 'open',
  });
}

function protectedCluster() {
  useContextsStore.getState().setList({
    current: { kubeconfig: '', name: 'p-mk1' },
    kubeconfigs: [],
    protection: 'protected',
  });
}

function renderPanel(active = true) {
  const onSelectResource = vi.fn();
  render(<GitopsAppPanel target={target} active={active} onSelectResource={onSelectResource} />);
  return { onSelectResource };
}

function lastActionCall(): { url: string; body: unknown } {
  const mock = globalThis.fetch as ReturnType<typeof vi.fn>;
  const call = [...mock.mock.calls]
    .reverse()
    .find((one) => String(one[0]).includes('/api/argocd/action'));
  const init = call?.[1] as RequestInit;
  const body = init.body;
  if (typeof body !== 'string') {
    throw new Error('the action carried no json body');
  }
  return { url: String(call?.[0]), body: JSON.parse(body) };
}

describe('the per-application panel', () => {
  beforeEach(() => {
    showModal.mockClear();
    close.mockClear();
    HTMLDialogElement.prototype.showModal = showModal;
    HTMLDialogElement.prototype.close = close;
    openCluster();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('says what to select when nothing is', () => {
    serve(appWith());
    render(<GitopsAppPanel target={null} onSelectResource={vi.fn()} />);

    expect(screen.getByText('Select an Argo application or a Flux applier.')).toBeInTheDocument();
  });

  it('reads nothing while another panel is the one showing', async () => {
    const fetchMock = serve(appWith({ resources: [deployment] }));
    renderPanel(false);

    await waitFor(() => {
      expect(screen.getByText('Loading the application')).toBeInTheDocument();
    });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('splits configuration from deployment state', async () => {
    serve(appWith());
    renderPanel();

    expect(await screen.findByText('https://example.test/apps')).toBeInTheDocument();
    expect(screen.getByText('OutOfSync')).toBeInTheDocument();
    expect(screen.getByText('abc1234')).toBeInTheDocument();
    expect(screen.getByText('auto · prune')).toBeInTheDocument();
  });

  it('names the sync mode alone when there is no policy', async () => {
    const app = appWith();
    app.source.policy = '';
    serve(app);
    renderPanel();

    expect(await screen.findByText('auto')).toBeInTheDocument();
  });

  it('shows the issues the backend found', async () => {
    serve(
      appWith({
        issues: [
          { severity: 'degraded', title: 'The last operation failed', detail: 'because' },
          { severity: 'info', title: 'Something else' },
        ],
      }),
    );
    renderPanel();

    expect(await screen.findByText('The last operation failed')).toBeInTheDocument();
    expect(screen.getByText('because')).toBeInTheDocument();
    expect(screen.getByText('Something else')).toBeInTheDocument();
  });

  it('keeps an issue whose severity the wire invented', async () => {
    serve(
      appWith({
        issues: [{ severity: 'odd', title: 'Something else' } as unknown as GitopsIssue],
      }),
    );
    renderPanel();

    expect(await screen.findByText('Something else')).toBeInTheDocument();
  });

  it('falls back to a plain message for a rejection that is not an Error', async () => {
    const user = userEvent.setup();
    const refuse = vi.fn<() => Promise<Response>>().mockRejectedValue('nope');
    const fetchMock = vi.fn((url: string) => {
      if (url.includes('/api/argocd/action')) {
        return refuse();
      }
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve(appWith({ resources: [deployment] })),
      });
    });
    vi.stubGlobal('fetch', fetchMock);
    renderPanel();

    await user.click(await screen.findByLabelText('Mark Deployment web'));
    await user.click(screen.getByRole('button', { name: 'Sync 1 marked' }));

    expect(await screen.findByText('action failed')).toBeInTheDocument();
  });

  it('reports what went wrong when the application cannot be read', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 400,
        json: () => Promise.resolve({ message: 'not an applier' }),
      }),
    );
    renderPanel();

    expect(await screen.findByText('not an applier')).toBeInTheDocument();
  });

  it('lists managed resources with their drift and events', async () => {
    serve(appWith({ resources: [{ ...deployment, eventsTruncated: true }] }));
    renderPanel();

    expect(await screen.findByText('spec.replicas')).toBeInTheDocument();
    expect(screen.getByText('1')).toBeInTheDocument();
    expect(screen.getByText('3')).toBeInTheDocument();
    expect(screen.getByText('Scaled')).toBeInTheDocument();
    expect(screen.getByText('More recent events are available.')).toBeInTheDocument();
  });

  it('names the writer that took a field on a server-side applied resource', async () => {
    serve(
      appWith({
        resources: [
          {
            ...deployment,
            drift: [{ path: 'spec.replicas', declared: 'argocd-controller', live: 'kubectl-edit' }],
            driftOwners: true,
            driftNote: 'this object is applied server-side',
          },
        ],
      }),
    );
    renderPanel();

    expect(await screen.findByText('spec.replicas')).toBeInTheDocument();
    expect(screen.getByText('argocd-controller')).toBeInTheDocument();
    expect(screen.getByText('kubectl-edit')).toBeInTheDocument();
    expect(screen.getByText('this object is applied server-side')).toBeInTheDocument();
  });

  it('explains why there is no drift to show', async () => {
    serve(
      appWith({
        resources: [{ ...deployment, drift: [], driftNote: 'no last-applied-configuration' }],
      }),
    );
    renderPanel();

    expect(await screen.findByText('no last-applied-configuration')).toBeInTheDocument();
  });

  it('marks a resource that is going away', async () => {
    serve(
      appWith({
        resources: [{ ...deployment, terminating: true, finalizers: ['foregroundDeletion'] }],
      }),
    );
    renderPanel();

    expect(await screen.findByText('Terminating')).toBeInTheDocument();
    expect(screen.getByText('held by foregroundDeletion')).toBeInTheDocument();
  });

  it('opens a managed resource', async () => {
    const user = userEvent.setup();
    serve(appWith({ resources: [deployment] }));
    const { onSelectResource } = renderPanel();

    await user.click(await screen.findByRole('button', { name: 'web' }));

    expect(onSelectResource).toHaveBeenCalledWith({
      group: 'apps',
      version: 'v1',
      resource: 'deployments',
      namespace: 'shop',
      name: 'web',
    });
  });

  it('leaves a resource unopenable when its kind is unknown', async () => {
    serve(appWith({ resources: [{ ...deployment, resource: undefined }] }));
    renderPanel();

    await screen.findByText('spec.replicas');
    expect(screen.queryByRole('button', { name: 'web' })).not.toBeInTheDocument();
  });

  it('shows nothing extra for a resource with no drift and no events', async () => {
    serve(
      appWith({
        resources: [{ ...deployment, drift: [], driftNote: '', events: [] }],
      }),
    );
    renderPanel();

    await screen.findByRole('button', { name: 'web' });
    expect(screen.queryByText('spec.replicas')).not.toBeInTheDocument();
    expect(screen.queryByText('Scaled')).not.toBeInTheDocument();
  });

  it('opens the object behind a node in the graph', async () => {
    const user = userEvent.setup();
    serve(appWith(), {
      nodes: [
        {
          id: 'a',
          kind: 'Deployment',
          group: 'apps',
          version: 'v1',
          resource: 'deployments',
          name: 'web',
          namespace: 'shop',
          status: 'Synced',
          ready: 'True',
          category: 'managed',
        },
      ],
      edges: [],
    });
    const { onSelectResource } = renderPanel();

    await user.click(await screen.findByRole('button', { name: 'Topology' }));
    await user.click(await screen.findByRole('button', { name: 'web' }));

    expect(onSelectResource).toHaveBeenCalledWith({
      group: 'apps',
      version: 'v1',
      resource: 'deployments',
      namespace: 'shop',
      name: 'web',
    });
  });

  it('retries a graph that stopped updating', async () => {
    const user = userEvent.setup();
    let graphCalls = 0;
    const node = {
      id: 'a',
      kind: 'Deployment',
      group: 'apps',
      version: 'v1',
      resource: 'deployments',
      name: 'web',
      namespace: 'shop',
      status: 'Synced',
      ready: 'True',
      category: 'managed',
    };
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/api/gitops/app/graph')) {
          graphCalls += 1;
          if (graphCalls === 1) {
            return Promise.resolve({
              ok: true,
              json: () => Promise.resolve({ nodes: [node], edges: [] }),
            });
          }
          return Promise.resolve({
            ok: false,
            status: 500,
            json: () => Promise.resolve({ message: 'no graph' }),
          });
        }
        if (url.includes('/api/gitops/app')) {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(appWith({ resources: [deployment] })),
          });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({ action: 'sync' }) });
      }),
    );
    renderPanel();

    await user.click(await screen.findByLabelText('Mark Deployment web'));
    await user.click(screen.getByRole('button', { name: 'Topology' }));
    await screen.findByRole('button', { name: 'web' });
    await user.click(screen.getByRole('button', { name: 'Sync 1 marked' }));
    await user.click(await screen.findByRole('button', { name: 'Retry' }));

    await waitFor(() => {
      expect(graphCalls).toBeGreaterThan(2);
    });
  });

  it('hides the previous topology while an action refreshes it', async () => {
    const user = userEvent.setup();
    const pending = new Promise<never>(() => undefined);
    let graphCalls = 0;
    const node = {
      id: 'a',
      kind: 'Deployment',
      group: 'apps',
      version: 'v1',
      resource: 'deployments',
      name: 'web',
      namespace: 'shop',
      status: 'Synced',
      ready: 'True',
      category: 'managed',
    };
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/api/gitops/app/graph')) {
          graphCalls += 1;
          if (graphCalls === 1) {
            return Promise.resolve({
              ok: true,
              json: () => Promise.resolve({ nodes: [node], edges: [] }),
            });
          }
          return pending;
        }
        if (url.includes('/api/gitops/app')) {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(appWith({ resources: [deployment] })),
          });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({ action: 'sync' }) });
      }),
    );
    renderPanel();

    await user.click(await screen.findByLabelText('Mark Deployment web'));
    await user.click(screen.getByRole('button', { name: 'Topology' }));
    await screen.findByRole('button', { name: 'web' });
    await user.click(screen.getByRole('button', { name: 'Sync 1 marked' }));

    await waitFor(() => {
      expect(graphCalls).toBe(2);
    });
    expect(screen.queryByRole('button', { name: 'web' })).not.toBeInTheDocument();
  });

  it('stops the writes while the application is being deleted', async () => {
    const user = userEvent.setup();
    serve(
      appWith({
        terminating: true,
        resources: [deployment],
        history: [{ id: 3, revision: 'ccc', deployedAt: 'then' }],
      }),
    );
    renderPanel();

    await user.click(await screen.findByLabelText('Mark Deployment web'));
    expect(screen.getByRole('button', { name: 'Sync 1 marked' })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Activity' }));
    expect(screen.getByRole('button', { name: 'Roll back' })).toBeDisabled();
  });

  it('says so when nothing is managed', async () => {
    serve(appWith());
    renderPanel();

    expect(
      await screen.findByText('This object records no managed resources.'),
    ).toBeInTheDocument();
  });

  it('syncs only the resources that were marked', async () => {
    const user = userEvent.setup();
    serve(appWith({ resources: [deployment] }));
    renderPanel();

    await user.click(await screen.findByLabelText('Mark Deployment web'));
    await user.click(screen.getByRole('button', { name: 'Sync 1 marked' }));

    await waitFor(() => {
      expect(lastActionCall().url).toContain('action=sync');
    });
    expect(lastActionCall().body).toEqual({
      resources: [{ group: 'apps', kind: 'Deployment', name: 'web', namespace: 'shop' }],
    });
    expect(await screen.findByText('Sync requested.')).toBeInTheDocument();
  });

  it('unmarks a resource that was marked', async () => {
    const user = userEvent.setup();
    serve(appWith({ resources: [deployment] }));
    renderPanel();

    const box = await screen.findByLabelText('Mark Deployment web');
    await user.click(box);
    await user.click(box);

    expect(screen.queryByRole('button', { name: /marked/ })).not.toBeInTheDocument();
  });

  it('reports what the server said about a failed action', async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn((url: string) => {
      if (url.includes('/api/argocd/action')) {
        return Promise.resolve({
          ok: false,
          status: 409,
          json: () => Promise.resolve({ message: 'podinfo syncs itself' }),
        });
      }
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve(appWith({ resources: [deployment] })),
      });
    });
    vi.stubGlobal('fetch', fetchMock);
    renderPanel();

    await user.click(await screen.findByLabelText('Mark Deployment web'));
    await user.click(screen.getByRole('button', { name: 'Sync 1 marked' }));

    expect(await screen.findByText('podinfo syncs itself')).toBeInTheDocument();
  });

  it('shows the operation and the deployments behind it', async () => {
    const user = userEvent.setup();
    serve(
      appWith({
        operation: { phase: 'Failed', message: 'it broke', cause: 'a webhook said no' },
        history: [
          { id: 0, revision: 'aaa', deployedAt: 'then', automated: true },
          { id: 1, revision: 'bbb', deployedAt: 'later', initiatedBy: 'someone' },
        ],
      }),
    );
    renderPanel();

    await user.click(await screen.findByRole('button', { name: 'Activity' }));

    expect(screen.getByText('Failed')).toBeInTheDocument();
    expect(screen.getByText('a webhook said no')).toBeInTheDocument();
    expect(screen.getByText('it broke')).toBeInTheDocument();
    expect(screen.getByText('automation')).toBeInTheDocument();
    expect(screen.getByText('someone')).toBeInTheDocument();
  });

  it('says so when nothing has been deployed yet', async () => {
    const user = userEvent.setup();
    serve(appWith());
    renderPanel();

    await user.click(await screen.findByRole('button', { name: 'Activity' }));

    expect(screen.getByText('No deployments recorded yet.')).toBeInTheDocument();
  });

  it('rolls back to a deployment in the history', async () => {
    const user = userEvent.setup();
    serve(appWith({ history: [{ id: 3, revision: 'ccc', deployedAt: 'then' }] }));
    renderPanel();

    await user.click(await screen.findByRole('button', { name: 'Activity' }));
    await user.click(screen.getByRole('button', { name: 'Roll back' }));

    await waitFor(() => {
      expect(lastActionCall().url).toContain('action=rollback');
    });
    expect(lastActionCall().body).toEqual({ revision: 3 });
    expect(await screen.findByText('Rollback requested.')).toBeInTheDocument();
  });

  it('offers no rollback for a flux applier', async () => {
    const user = userEvent.setup();
    serve(
      appWith({
        controller: 'flux',
        history: [{ id: 3, revision: '6.7.0', deployedAt: 'then' }],
      }),
    );
    renderPanel();

    await user.click(await screen.findByRole('button', { name: 'Activity' }));

    expect(screen.queryByRole('button', { name: 'Roll back' })).not.toBeInTheDocument();
  });

  it('draws the managed resources as a graph', async () => {
    const user = userEvent.setup();
    serve(appWith({ resources: [deployment] }), {
      nodes: [
        {
          id: 'a',
          kind: 'Application',
          group: 'argoproj.io',
          version: 'v1alpha1',
          resource: 'applications',
          name: 'podinfo',
          namespace: 'argocd',
          status: 'Synced',
          ready: 'True',
          category: 'app',
        },
      ],
      edges: [],
    });
    renderPanel();

    await user.click(await screen.findByRole('button', { name: 'Topology' }));

    await waitFor(() => {
      expect(screen.getByText('Manages')).toBeInTheDocument();
    });
  });

  it('reports a graph that could not be read', async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn((url: string) => {
      if (url.includes('/api/gitops/app/graph')) {
        return Promise.resolve({
          ok: false,
          status: 500,
          json: () => Promise.resolve({ message: 'no graph' }),
        });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve(appWith()) });
    });
    vi.stubGlobal('fetch', fetchMock);
    renderPanel();

    await user.click(await screen.findByRole('button', { name: 'Topology' }));

    expect(await screen.findByText(/no graph/)).toBeInTheDocument();
  });
});

describe('the per-application panel on a protected cluster', () => {
  beforeEach(() => {
    showModal.mockClear();
    close.mockClear();
    HTMLDialogElement.prototype.showModal = showModal;
    HTMLDialogElement.prototype.close = close;
    protectedCluster();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    openCluster();
  });

  it('asks for the name before syncing marked resources', async () => {
    const user = userEvent.setup();
    serve(appWith({ resources: [deployment] }));
    renderPanel();

    await user.click(await screen.findByLabelText('Mark Deployment web'));
    await user.click(screen.getByRole('button', { name: 'Sync 1 marked' }));

    expect(screen.getByText('Syncing one marked resource of podinfo.')).toBeInTheDocument();
  });

  it('counts the marked resources in the question', async () => {
    const user = userEvent.setup();
    serve(appWith({ resources: [deployment, { ...deployment, name: 'api' }] }));
    renderPanel();

    await user.click(await screen.findByLabelText('Mark Deployment web'));
    await user.click(screen.getByLabelText('Mark Deployment api'));
    await user.click(screen.getByRole('button', { name: 'Sync 2 marked' }));

    expect(screen.getByText('Syncing 2 marked resources of podinfo.')).toBeInTheDocument();
  });

  it('asks for the name before rolling back', async () => {
    const user = userEvent.setup();
    serve(appWith({ history: [{ id: 3, revision: 'ccc', deployedAt: 'then' }] }));
    renderPanel();

    await user.click(await screen.findByRole('button', { name: 'Activity' }));
    await user.click(screen.getByRole('button', { name: 'Roll back' }));

    expect(screen.getByText('Rolling podinfo back to deployment 3.')).toBeInTheDocument();
  });

  it('rolls back once the name is typed', async () => {
    const user = userEvent.setup();
    serve(appWith({ history: [{ id: 3, revision: 'ccc', deployedAt: 'then' }] }));
    renderPanel();
    await user.click(await screen.findByRole('button', { name: 'Activity' }));
    await user.click(screen.getByRole('button', { name: 'Roll back' }));

    await user.type(screen.getByLabelText('Name'), 'podinfo');
    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    await waitFor(() => {
      expect(lastActionCall().url).toContain('confirm=podinfo');
    });
  });

  it('drops the question when it is cancelled', async () => {
    const user = userEvent.setup();
    const fetchMock = serve(appWith({ history: [{ id: 3, revision: 'ccc', deployedAt: 'then' }] }));
    renderPanel();
    await user.click(await screen.findByRole('button', { name: 'Activity' }));
    await user.click(screen.getByRole('button', { name: 'Roll back' }));

    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(screen.queryByText('Rolling podinfo back to deployment 3.')).not.toBeInTheDocument();
    expect(fetchMock.mock.calls.every((one) => !one[0].includes('/action'))).toBe(true);
  });
});

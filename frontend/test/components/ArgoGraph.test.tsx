import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ArgoGraph from '../../src/components/ArgoGraph';
import type { ArgoApp, GraphNode } from '../../src/lib/types';

vi.mock('@xyflow/react', () => ({
  ReactFlow: ({
    nodes,
    edges,
    onNodeClick,
  }: {
    nodes: { id: string; data: { label: string; node: GraphNode } }[];
    edges: { id: string }[];
    onNodeClick: (event: unknown, node: { data: { node: GraphNode } }) => void;
  }) => (
    <div data-testid="react-flow" data-edges={edges.map((edge) => edge.id).join(',')}>
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

function makeApp(name: string, extra: Partial<ArgoApp> = {}): ArgoApp {
  return {
    kind: 'Application',
    group: 'argoproj.io',
    version: 'v1alpha1',
    resource: 'applications',
    name,
    namespace: 'argocd',
    project: 'default',
    sync: 'Synced',
    health: 'Healthy',
    revision: 'abc1234',
    repo: 'https://git/apps',
    path: `apps/${name}`,
    destination: 'in-cluster shop',
    message: '',
    createdAt: '2026-08-17T09:00:00Z',
    ...extra,
  };
}

function stub(body: unknown, ok = true) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.resolve({ ok, status: ok ? 200 : 500, json: () => Promise.resolve(body) })),
  );
}

function show(body: Record<string, unknown>, onSelect = vi.fn()) {
  stub({ apps: [], applicationSets: [], projects: [], ...body });
  render(<ArgoGraph onSelect={onSelect} />);
  return { onSelect };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('ArgoGraph', () => {
  it('says it is loading before the first answer', () => {
    show({});

    expect(screen.getByText('Loading graph')).toBeInTheDocument();
  });

  it('draws an app of apps as an edge from parent to child', async () => {
    show({ apps: [makeApp('root'), makeApp('web', { owner: 'root' })] });

    expect(await screen.findByRole('button', { name: 'root' })).toBeInTheDocument();
    expect(screen.getByTestId('react-flow')).toHaveAttribute(
      'data-edges',
      'applications/argocd/root->applications/argocd/web:manages',
    );
  });

  it('draws an application set above the applications it generates', async () => {
    show({
      apps: [makeApp('web', { owner: 'shops' })],
      applicationSets: [
        makeApp('shops', { kind: 'ApplicationSet', resource: 'applicationsets', sync: '' }),
      ],
    });

    expect(await screen.findByRole('button', { name: 'shops' })).toBeInTheDocument();
    expect(screen.getByTestId('react-flow')).toHaveAttribute(
      'data-edges',
      'applicationsets/argocd/shops->applications/argocd/web:manages',
    );
  });

  it('leaves an app whose parent is not in the graph unattached', async () => {
    show({ apps: [makeApp('web', { owner: 'gone' })] });

    await screen.findByRole('button', { name: 'web' });

    expect(screen.getByTestId('react-flow')).toHaveAttribute('data-edges', '');
  });

  it('opens the object behind a node', async () => {
    const user = userEvent.setup();
    const { onSelect } = show({ apps: [makeApp('web')] });

    await user.click(await screen.findByRole('button', { name: 'web' }));

    expect(onSelect).toHaveBeenCalledWith(
      expect.objectContaining({ name: 'web', resource: 'applications' }),
    );
  });

  it('says so when there is nothing to draw', async () => {
    show({});

    expect(await screen.findByText('No Argo CD applications on this cluster.')).toBeInTheDocument();
  });

  it('leaves the flux source tone out of the legend', async () => {
    show({ apps: [makeApp('web')] });

    await screen.findByRole('button', { name: 'web' });

    expect(screen.getByText('Ready')).toBeInTheDocument();
    expect(screen.queryByText('Source, not ready yet')).not.toBeInTheDocument();
  });

  it('names only the edge kind Argo uses', async () => {
    show({ apps: [makeApp('web')] });

    await screen.findByRole('button', { name: 'web' });

    expect(screen.getByText('Manages')).toBeInTheDocument();
    expect(screen.queryByText('Depends on')).not.toBeInTheDocument();
    expect(screen.queryByText('Source')).not.toBeInTheDocument();
  });

  it('shows the request failure when nothing has arrived', async () => {
    stub({ message: 'argo is unreachable' }, false);

    render(<ArgoGraph />);

    expect(await screen.findByText(/argo is unreachable/)).toBeInTheDocument();
  });
});

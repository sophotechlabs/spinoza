import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { Graph } from '../../src/lib/types';
import type { GitopsFlowNode } from '../../src/lib/graphLayout';
import { makeGraphEdge, makeGraphNode } from '../helpers';

vi.mock('@xyflow/react', () => {
  const ReactFlowStub = ({
    nodes,
    edges,
    onNodeClick,
    children,
  }: {
    nodes: GitopsFlowNode[];
    edges: { id: string }[];
    onNodeClick?: (event: unknown, node: GitopsFlowNode) => void;
    children?: ReactNode;
  }) => (
    <div data-testid="react-flow" data-edges={edges.length}>
      {nodes.map((node) => (
        <button
          key={node.id}
          type="button"
          onClick={(event) => {
            if (onNodeClick) {
              onNodeClick(event, node);
            }
          }}
        >
          {node.data.label}
        </button>
      ))}
      {children}
    </div>
  );
  const Background = () => <div data-testid="background" />;
  const Controls = () => <div data-testid="controls" />;
  return { ReactFlow: ReactFlowStub, Background, Controls };
});

import TopologyGraph from '../../src/components/TopologyGraph';
import { useNamespaceStore } from '../../src/store/namespace';

const folded = makeGraphNode({
  id: 'dep-api',
  kind: 'Deployment',
  group: 'apps',
  resource: 'deployments',
  name: 'api',
  namespace: 'prod',
  category: 'workload',
  contains: 3,
  unhealthy: 1,
  ready: 'False',
});

const opened = makeGraphNode({
  id: 'rs-api',
  kind: 'ReplicaSet',
  group: 'apps',
  resource: 'replicasets',
  name: 'api-abc',
  namespace: 'prod',
  category: 'workload',
  contains: 2,
});

const leaf = makeGraphNode({
  id: 'svc-api',
  kind: 'Service',
  group: '',
  resource: 'services',
  name: 'api',
  namespace: 'prod',
  category: 'service',
});

function urlsFor(graphs: Record<string, Graph>, fallback: Graph): string[] {
  const seen: string[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      seen.push(url);
      const match = Object.keys(graphs).find((part) => url.includes(part));
      let body = fallback;
      if (match !== undefined) {
        body = graphs[match];
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve(body) });
    }),
  );
  return seen;
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
  act(() => {
    useNamespaceStore.getState().reset();
  });
});

describe('TopologyGraph', () => {
  it('says how much a folded node is hiding', async () => {
    urlsFor({}, { nodes: [folded], edges: [] });

    render(<TopologyGraph openedOn={null} />);

    expect(await screen.findByText('api ×3 · 1 not ready')).toBeInTheDocument();
  });

  it('counts without alarming when everything inside is well', async () => {
    urlsFor({}, { nodes: [opened], edges: [] });

    render(<TopologyGraph openedOn={null} />);

    expect(await screen.findByText('api-abc ×2')).toBeInTheDocument();
  });

  it('opens a folded node instead of the drawer', async () => {
    const onSelect = vi.fn();
    const seen = urlsFor(
      { 'expand=dep-api': { nodes: [opened], edges: [] } },
      {
        nodes: [folded],
        edges: [],
      },
    );

    render(<TopologyGraph openedOn={null} onSelect={onSelect} />);
    await userEvent.click(await screen.findByText('api ×3 · 1 not ready'));

    expect(await screen.findByText('api-abc ×2')).toBeInTheDocument();
    expect(onSelect).not.toHaveBeenCalled();
    expect(seen.some((url) => url.includes('expand=dep-api'))).toBe(true);
  });

  it('closes a node that is already open', async () => {
    const onSelect = vi.fn();
    const unfolded = makeGraphNode({ ...folded, contains: 0, unhealthy: 0 });
    urlsFor(
      { 'expand=dep-api': { nodes: [unfolded, opened], edges: [] } },
      {
        nodes: [folded],
        edges: [],
      },
    );

    render(<TopologyGraph openedOn={null} onSelect={onSelect} />);
    await userEvent.click(await screen.findByText('api ×3 · 1 not ready'));
    await screen.findByText('api-abc ×2');
    await userEvent.click(screen.getByText('api'));

    expect(await screen.findByText('api ×3 · 1 not ready')).toBeInTheDocument();
    expect(onSelect).not.toHaveBeenCalled();
  });

  it('opens the drawer for a node with nothing left inside it', async () => {
    const onSelect = vi.fn();
    urlsFor({}, { nodes: [leaf], edges: [makeGraphEdge({ from: 'svc-api', to: 'svc-api' })] });

    render(<TopologyGraph openedOn={null} onSelect={onSelect} />);
    await userEvent.click(await screen.findByText('api'));

    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: 'svc-api' }));
  });

  it('does not throw when a leaf is clicked with no drawer to open', async () => {
    urlsFor({}, { nodes: [leaf], edges: [] });

    render(<TopologyGraph openedOn={null} />);
    await userEvent.click(await screen.findByText('api'));

    expect(screen.getByText('api')).toBeInTheDocument();
  });

  it('asks for the namespace the picker is on', async () => {
    const seen = urlsFor({}, { nodes: [folded], edges: [] });
    act(() => {
      useNamespaceStore.getState().choose('prod');
    });

    render(<TopologyGraph openedOn={null} />);
    await screen.findByText('api ×3 · 1 not ready');

    expect(seen.some((url) => url.includes('namespace=prod'))).toBe(true);
  });

  it('names the namespace when it holds nothing', async () => {
    urlsFor({}, { nodes: [], edges: [] });
    act(() => {
      useNamespaceStore.getState().choose('prod');
    });

    render(<TopologyGraph openedOn={null} />);

    expect(await screen.findByText('No workloads found in prod.')).toBeInTheDocument();
  });

  it('speaks of the cluster when no namespace is chosen', async () => {
    urlsFor({}, { nodes: [], edges: [] });

    render(<TopologyGraph openedOn={null} />);

    expect(await screen.findByText('No workloads found in this cluster.')).toBeInTheDocument();
  });

  it('says which namespace to narrow to when the fold is still too much', async () => {
    const many = Array.from({ length: 401 }, (_unused, index) =>
      makeGraphNode({ id: `n-${index}`, name: `node-${index}`, namespace: 'prod' }),
    );
    urlsFor({}, { nodes: many, edges: [] });

    render(<TopologyGraph openedOn={null} />);

    expect(await screen.findByText(/prod is the biggest/)).toBeInTheDocument();
  });

  it('just asks for less when nothing on the graph is in a namespace', async () => {
    const many = Array.from({ length: 401 }, (_unused, index) =>
      makeGraphNode({ id: `n-${index}`, name: `node-${index}`, namespace: '' }),
    );
    urlsFor({}, { nodes: many, edges: [] });

    render(<TopologyGraph openedOn={null} />);

    expect(await screen.findByText(/Narrow the view down/)).toBeInTheDocument();
  });

  it('lets go of the root and the open nodes when the namespace changes', async () => {
    const seen = urlsFor({}, { nodes: [folded], edges: [] });
    act(() => {
      useNamespaceStore.getState().choose('prod');
    });

    render(
      <TopologyGraph
        openedOn={{
          group: 'apps',
          version: 'v1',
          resource: 'deployments',
          namespace: 'prod',
          name: 'api',
        }}
      />,
    );
    await userEvent.click(await screen.findByText('api ×3 · 1 not ready'));
    await screen.findByText('api ×3 · 1 not ready');
    seen.length = 0;
    act(() => {
      useNamespaceStore.getState().choose('staging');
    });
    await screen.findByText('api ×3 · 1 not ready');

    expect(screen.queryByText('Around deployments/api')).not.toBeInTheDocument();
    expect(seen.every((url) => !url.includes('rootName'))).toBe(true);
    expect(seen.every((url) => !url.includes('expand'))).toBe(true);
  });

  it('opens around the object it was reached from, and widens on demand', async () => {
    const seen = urlsFor({}, { nodes: [folded], edges: [] });

    render(
      <TopologyGraph
        openedOn={{
          group: 'apps',
          version: 'v1',
          resource: 'deployments',
          namespace: 'prod',
          name: 'api',
        }}
      />,
    );
    await screen.findByText('api ×3 · 1 not ready');
    expect(seen.some((url) => url.includes('rootName=api'))).toBe(true);

    await userEvent.click(screen.getByRole('button', { name: 'Show the whole scope' }));

    expect(screen.queryByText('Around deployments/api')).not.toBeInTheDocument();
  });
});

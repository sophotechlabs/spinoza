import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { Graph } from '../../src/lib/types';
import type { GitopsFlowNode } from '../../src/lib/graphLayout';
import { makeGraphEdge, makeGraphNode } from '../helpers';

const fitViewSpy = vi.fn();

vi.mock('@xyflow/react', () => {
  const ReactFlowStub = ({
    nodes,
    edges,
    onNodeClick,
    children,
    minZoom,
  }: {
    nodes: GitopsFlowNode[];
    edges: { id: string }[];
    onNodeClick?: (event: unknown, node: GitopsFlowNode) => void;
    children?: ReactNode;
    minZoom?: number;
  }) => (
    <div
      data-testid="react-flow"
      data-edges={edges.length}
      data-min-zoom={minZoom}
      data-sized={
        nodes.filter((node) => node.width !== undefined && node.height !== undefined).length
      }
    >
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
  const useReactFlow = () => ({ fitView: fitViewSpy });
  return { ReactFlow: ReactFlowStub, Background, Controls, useReactFlow };
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
  fitViewSpy.mockClear();
  vi.unstubAllGlobals();
  vi.useRealTimers();
  act(() => {
    useNamespaceStore.getState().reset();
  });
});

describe('TopologyGraph', () => {
  it("lets fitView zoom below React Flow's default floor", async () => {
    urlsFor({}, { nodes: [leaf], edges: [] });

    render(<TopologyGraph openedOn={null} />);

    expect(await screen.findByTestId('react-flow')).toHaveAttribute('data-min-zoom', '0.01');
  });

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

  it('warns above the graph when only some resource types could be listed', async () => {
    urlsFor(
      {},
      {
        nodes: [folded],
        edges: [],
        error:
          '1 of 11 resource types could not be listed: ingresses.networking.k8s.io (is forbidden)',
      },
    );

    render(<TopologyGraph openedOn={null} />);

    expect(await screen.findByText(/ingresses.networking.k8s.io/)).toBeInTheDocument();
    expect(screen.getByTestId('react-flow')).toBeInTheDocument();
  });

  it('says the graph could not be loaded when nothing came back at all', async () => {
    urlsFor(
      {},
      {
        nodes: [],
        edges: [],
        error: '11 of 11 resource types could not be listed: pods (is forbidden)',
      },
    );

    render(<TopologyGraph openedOn={null} />);

    expect(await screen.findByText('The topology graph could not be loaded')).toBeInTheDocument();
    expect(screen.getByText(/pods \(is forbidden\)/)).toBeInTheDocument();
  });

  it('keeps the last graph and says it stopped updating once a poll fails', async () => {
    vi.useFakeTimers();
    let call = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(() => {
        call += 1;
        if (call === 1) {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: [folded], edges: [] }),
          });
        }
        return Promise.reject(new Error('the topology endpoint is down'));
      }),
    );

    render(<TopologyGraph openedOn={null} />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByText('api ×3 · 1 not ready')).toBeInTheDocument();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });

    expect(screen.getByRole('status')).toHaveTextContent('the topology endpoint is down');
    expect(screen.getByText('api ×3 · 1 not ready')).toBeInTheDocument();
  });

  it('says so when the request fails with something that is not an error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('boom'));

    render(<TopologyGraph openedOn={null} />);

    expect(await screen.findByText('topology request failed')).toBeInTheDocument();
  });

  it('re-asks on the poll interval and redraws what changed', async () => {
    vi.useFakeTimers();
    const grown = makeGraphNode({ ...folded, contains: 4, unhealthy: 0, ready: 'True' });
    let call = 0;
    const fetchMock = vi.fn(() => {
      call += 1;
      const nodes = [folded];
      if (call > 1) {
        nodes[0] = grown;
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve({ nodes, edges: [] }) });
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<TopologyGraph openedOn={null} />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByText('api ×3 · 1 not ready')).toBeInTheDocument();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });

    expect(screen.getByText('api ×4')).toBeInTheDocument();
    expect(fetchMock.mock.calls.length).toBeGreaterThan(1);
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

  it('does not draw the previous namespace while the next graph loads', async () => {
    let finishNext: (response: { ok: boolean; json: () => Promise<unknown> }) => void = () =>
      undefined;
    const fetchMock = vi.fn((url: string) => {
      if (url.includes('namespace=staging')) {
        return new Promise<{ ok: boolean; json: () => Promise<unknown> }>((resolve) => {
          finishNext = resolve;
        });
      }
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ nodes: [folded], edges: [] }),
      });
    });
    vi.stubGlobal('fetch', fetchMock);
    act(() => {
      useNamespaceStore.getState().choose('prod');
    });
    render(<TopologyGraph openedOn={null} />);
    expect(await screen.findByText('api ×3 · 1 not ready')).toBeInTheDocument();

    act(() => {
      useNamespaceStore.getState().choose('staging');
    });
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2);
    });
    expect(screen.queryByText('api ×3 · 1 not ready')).not.toBeInTheDocument();
    expect(screen.getByText('Loading graph')).toBeInTheDocument();

    await act(async () => {
      finishNext({ ok: true, json: () => Promise.resolve({ nodes: [leaf], edges: [] }) });
      await Promise.resolve();
    });
    expect(await screen.findByText('api')).toBeInTheDocument();
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

describe('the canvas fits itself to the graph it was given', () => {
  it('fits once the graph has arrived, not only on an empty mount', async () => {
    urlsFor(
      {},
      { nodes: [folded, leaf], edges: [makeGraphEdge({ from: 'dep-api', to: 'svc-api' })] },
    );

    render(<TopologyGraph openedOn={null} />);
    await screen.findByText('api · Deployment ×3 · 1 not ready');

    expect(fitViewSpy).toHaveBeenCalled();
  });

  it('hands the edges to the canvas rather than dropping them', async () => {
    urlsFor(
      {},
      { nodes: [folded, leaf], edges: [makeGraphEdge({ from: 'dep-api', to: 'svc-api' })] },
    );

    render(<TopologyGraph openedOn={null} />);
    await screen.findByText('api · Deployment ×3 · 1 not ready');

    expect(screen.getByTestId('react-flow').getAttribute('data-edges')).toBe('1');
  });

  it('gives the canvas nodes an explicit size', async () => {
    urlsFor({}, { nodes: [folded, leaf], edges: [] });

    render(<TopologyGraph openedOn={null} />);
    await screen.findByText('api · Deployment ×3 · 1 not ready');

    expect(screen.getByTestId('react-flow').getAttribute('data-sized')).toBe('2');
  });
});

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

import GitopsGraph from '../../src/components/GitopsGraph';

function stubGraph(graph: Graph): void {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(graph) }),
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe('GitopsGraph', () => {
  it('shows a loading state before the graph resolves', async () => {
    stubGraph({ nodes: [], edges: [] });
    render(<GitopsGraph />);
    expect(screen.getByText('Loading graph…')).toBeInTheDocument();
    expect(await screen.findByText('No GitOps resources found.')).toBeInTheDocument();
  });

  it('renders the graph nodes, background and controls once loaded', async () => {
    const graph: Graph = {
      nodes: [
        makeGraphNode({ id: 'a', name: 'alpha' }),
        makeGraphNode({ id: 'b', name: 'bravo', category: 'app' }),
      ],
      edges: [makeGraphEdge({ from: 'a', to: 'b', kind: 'source' })],
    };
    stubGraph(graph);
    render(<GitopsGraph />);
    expect(await screen.findByText('alpha')).toBeInTheDocument();
    expect(screen.getByText('bravo')).toBeInTheDocument();
    expect(screen.getByTestId('background')).toBeInTheDocument();
    expect(screen.getByTestId('controls')).toBeInTheDocument();
    expect(screen.getByTestId('react-flow')).toHaveAttribute('data-edges', '1');
  });

  it('filters managed nodes out of the rendered control plane', async () => {
    const graph: Graph = {
      nodes: [
        makeGraphNode({ id: 'a', name: 'alpha', category: 'source' }),
        makeGraphNode({ id: 'z', name: 'zulu', category: 'managed' }),
      ],
      edges: [makeGraphEdge({ from: 'a', to: 'z', kind: 'manages' })],
    };
    stubGraph(graph);
    render(<GitopsGraph />);
    expect(await screen.findByText('alpha')).toBeInTheDocument();
    expect(screen.queryByText('zulu')).not.toBeInTheDocument();
    expect(screen.getByTestId('react-flow')).toHaveAttribute('data-edges', '0');
  });

  it('shows an over-limit message when the control plane exceeds the node cap', async () => {
    const nodes = Array.from({ length: 401 }, (_unused, index) =>
      makeGraphNode({ id: `n-${index}`, name: `node-${index}`, category: 'app' }),
    );
    stubGraph({ nodes, edges: [] });
    render(<GitopsGraph />);
    expect(await screen.findByText(/401 nodes/)).toBeInTheDocument();
    expect(screen.queryByTestId('react-flow')).not.toBeInTheDocument();
  });

  it('shows the error message when the fetch rejects with an error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('graph down')));
    render(<GitopsGraph />);
    expect(await screen.findByText('graph down')).toBeInTheDocument();
  });

  it('shows a generic message when the fetch rejects with a non-error value', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('boom'));
    render(<GitopsGraph />);
    expect(await screen.findByText('gitops graph request failed')).toBeInTheDocument();
  });

  it('calls onSelect with the node payload when a node is clicked', async () => {
    const onSelect = vi.fn();
    stubGraph({ nodes: [makeGraphNode({ id: 'a', name: 'alpha' })], edges: [] });
    render(<GitopsGraph onSelect={onSelect} />);
    await userEvent.click(await screen.findByText('alpha'));
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: 'a', name: 'alpha' }));
  });

  it('does not throw when a node is clicked without an onSelect handler', async () => {
    stubGraph({ nodes: [makeGraphNode({ id: 'a', name: 'alpha' })], edges: [] });
    render(<GitopsGraph />);
    await userEvent.click(await screen.findByText('alpha'));
    expect(screen.getByText('alpha')).toBeInTheDocument();
  });

  it('re-fetches and re-renders on the poll interval', async () => {
    vi.useFakeTimers();
    const graphA: Graph = { nodes: [makeGraphNode({ id: 'a', name: 'alpha' })], edges: [] };
    const graphB: Graph = { nodes: [makeGraphNode({ id: 'b', name: 'bravo' })], edges: [] };
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: () => Promise.resolve(graphA) })
      .mockResolvedValue({ ok: true, json: () => Promise.resolve(graphB) });
    vi.stubGlobal('fetch', fetchMock);
    render(<GitopsGraph />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByText('alpha')).toBeInTheDocument();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });
    expect(screen.getByText('bravo')).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});

describe('GitopsGraph partial failures', () => {
  it('says the graph could not be loaded when nothing came back', async () => {
    stubGraph({
      nodes: [],
      edges: [],
      error: '2 of 9 resource types could not be listed; buckets: is forbidden',
    });

    render(<GitopsGraph />);

    expect(await screen.findByText('The GitOps graph could not be loaded')).toBeInTheDocument();
    expect(screen.getByText(/buckets: is forbidden/)).toBeInTheDocument();
  });

  it('warns above the graph when only some lists failed', async () => {
    stubGraph({
      nodes: [makeGraphNode({ id: 'a', name: 'apps' })],
      edges: [],
      error: '1 of 9 resource types could not be listed; buckets: is forbidden',
    });

    render(<GitopsGraph />);

    expect(await screen.findByRole('status')).toHaveTextContent('buckets: is forbidden');
    expect(screen.getByTestId('react-flow')).toBeInTheDocument();
  });

  it('shows no warning when every list worked', async () => {
    stubGraph({ nodes: [makeGraphNode({ id: 'a', name: 'apps' })], edges: [] });

    render(<GitopsGraph />);

    await screen.findByTestId('react-flow');
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('still says no resources when the cluster simply has none', async () => {
    stubGraph({ nodes: [], edges: [] });

    render(<GitopsGraph />);

    expect(await screen.findByText('No GitOps resources found.')).toBeInTheDocument();
  });
});

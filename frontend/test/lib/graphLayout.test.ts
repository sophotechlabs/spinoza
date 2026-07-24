import { afterEach, describe, expect, it, vi } from 'vitest';
import dagre from '@dagrejs/dagre';
import type { Graph } from '../../src/lib/types';
import { controlPlane, toFlow } from '../../src/lib/graphLayout';
import type { GitopsFlowNode } from '../../src/lib/graphLayout';
import { makeGraphEdge, makeGraphNode } from '../helpers';

function nodeById(nodes: GitopsFlowNode[], id: string): GitopsFlowNode {
  const found = nodes.find((node) => node.id === id);
  if (!found) {
    throw new Error(`node ${id} not found`);
  }
  return found;
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('toFlow', () => {
  it('returns empty nodes and edges for an empty graph', () => {
    const flow = toFlow({ nodes: [], edges: [] });
    expect(flow.nodes).toEqual([]);
    expect(flow.edges).toEqual([]);
  });

  it('assigns a numeric layout position to every node', () => {
    const graph: Graph = {
      nodes: [
        makeGraphNode({ id: 'a', category: 'source' }),
        makeGraphNode({ id: 'b', category: 'app' }),
      ],
      edges: [makeGraphEdge({ from: 'a', to: 'b', kind: 'source' })],
    };
    const flow = toFlow(graph);
    for (const node of flow.nodes) {
      expect(typeof node.position.x).toBe('number');
      expect(typeof node.position.y).toBe('number');
      expect(Number.isNaN(node.position.x)).toBe(false);
      expect(Number.isNaN(node.position.y)).toBe(false);
    }
  });

  it('carries the source node label and payload into the flow node data', () => {
    const graph: Graph = {
      nodes: [makeGraphNode({ id: 'a', name: 'flux-system' })],
      edges: [],
    };
    const flow = toFlow(graph);
    const node = nodeById(flow.nodes, 'a');
    expect(node.data.label).toBe('flux-system');
    expect(node.data.node.name).toBe('flux-system');
  });

  it('maps a Ready status to the green style regardless of category', () => {
    const graph: Graph = {
      nodes: [makeGraphNode({ id: 'a', status: 'Ready', category: 'managed' })],
      edges: [],
    };
    const node = nodeById(toFlow(graph).nodes, 'a');
    expect(node.className).toContain('green');
  });

  it('maps a NotReady status to the red style', () => {
    const graph: Graph = {
      nodes: [makeGraphNode({ id: 'a', status: 'NotReady', category: 'app' })],
      edges: [],
    };
    const node = nodeById(toFlow(graph).nodes, 'a');
    expect(node.className).toContain('red');
  });

  it('maps a source category with a neutral status to the sky style', () => {
    const graph: Graph = {
      nodes: [makeGraphNode({ id: 'a', status: 'Unknown', category: 'source' })],
      edges: [],
    };
    const node = nodeById(toFlow(graph).nodes, 'a');
    expect(node.className).toContain('sky');
  });

  it('maps a managed category with a neutral status to the neutral style', () => {
    const graph: Graph = {
      nodes: [makeGraphNode({ id: 'a', status: 'Unknown', category: 'managed' })],
      edges: [],
    };
    const node = nodeById(toFlow(graph).nodes, 'a');
    expect(node.className).toContain('border-neutral-700');
  });

  it('maps an app category with a neutral status to the default style', () => {
    const graph: Graph = {
      nodes: [makeGraphNode({ id: 'a', status: 'Unknown', category: 'app' })],
      edges: [],
    };
    const node = nodeById(toFlow(graph).nodes, 'a');
    expect(node.className).toContain('border-neutral-600');
  });

  it('maps an applier category with a neutral status to the default style', () => {
    const graph: Graph = {
      nodes: [makeGraphNode({ id: 'a', status: 'Pending', category: 'applier' })],
      edges: [],
    };
    const node = nodeById(toFlow(graph).nodes, 'a');
    expect(node.className).toContain('border-neutral-600');
  });

  it('maps each edge kind to a stroke and animation state', () => {
    const graph: Graph = {
      nodes: [
        makeGraphNode({ id: 'a' }),
        makeGraphNode({ id: 'b' }),
        makeGraphNode({ id: 'c' }),
        makeGraphNode({ id: 'd' }),
      ],
      edges: [
        makeGraphEdge({ from: 'a', to: 'b', kind: 'source' }),
        makeGraphEdge({ from: 'b', to: 'c', kind: 'dependsOn' }),
        makeGraphEdge({ from: 'c', to: 'd', kind: 'manages' }),
      ],
    };
    const flow = toFlow(graph);
    const [source, depends, manages] = flow.edges;
    expect(source.id).toBe('a->b:source');
    expect(source.source).toBe('a');
    expect(source.target).toBe('b');
    expect(source.animated).toBe(true);
    expect(source.style).toEqual({ stroke: '#0ea5e9' });
    expect(depends.animated).toBe(false);
    expect(depends.style).toEqual({ stroke: '#f59e0b' });
    expect(manages.animated).toBe(false);
    expect(manages.style).toEqual({ stroke: '#525252' });
  });

  it('falls back to a zero position when layout leaves a node unpositioned', () => {
    vi.spyOn(dagre, 'layout').mockImplementation((g) => g);
    const graph: Graph = {
      nodes: [makeGraphNode({ id: 'a' })],
      edges: [],
    };
    const node = nodeById(toFlow(graph).nodes, 'a');
    expect(node.position).toEqual({ x: 0, y: 0 });
  });
});

describe('controlPlane', () => {
  it('drops managed nodes and every edge that touches one', () => {
    const graph: Graph = {
      nodes: [
        makeGraphNode({ id: 'a', category: 'source' }),
        makeGraphNode({ id: 'b', category: 'applier' }),
        makeGraphNode({ id: 'c', category: 'managed' }),
      ],
      edges: [
        makeGraphEdge({ from: 'a', to: 'b', kind: 'source' }),
        makeGraphEdge({ from: 'b', to: 'c', kind: 'manages' }),
      ],
    };
    const reduced = controlPlane(graph);
    expect(reduced.nodes.map((node) => node.id)).toEqual(['a', 'b']);
    expect(reduced.edges).toEqual([makeGraphEdge({ from: 'a', to: 'b', kind: 'source' })]);
  });

  it('keeps every node and edge when none are managed', () => {
    const graph: Graph = {
      nodes: [
        makeGraphNode({ id: 'a', category: 'source' }),
        makeGraphNode({ id: 'b', category: 'app' }),
      ],
      edges: [makeGraphEdge({ from: 'a', to: 'b', kind: 'source' })],
    };
    const reduced = controlPlane(graph);
    expect(reduced.nodes).toHaveLength(2);
    expect(reduced.edges).toHaveLength(1);
  });
});

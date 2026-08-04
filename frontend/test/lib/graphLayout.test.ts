import { afterEach, describe, expect, it, vi } from 'vitest';
import dagre from '@dagrejs/dagre';
import type { Graph } from '../../src/lib/types';
import { statusTone, toFlow } from '../../src/lib/graphLayout';
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
    expect(node.className).toContain('bg-ok-tint');
  });

  it('maps a not-ready object to the red style whatever the reason says', () => {
    const graph: Graph = {
      nodes: [makeGraphNode({ id: 'a', status: 'InstallFailed', ready: 'False', category: 'app' })],
      edges: [],
    };
    const node = nodeById(toFlow(graph).nodes, 'a');
    expect(node.className).toContain('bg-error-tint');
  });

  it('maps a source category with a neutral status to the sky style', () => {
    const graph: Graph = {
      nodes: [makeGraphNode({ id: 'a', status: 'Unknown', ready: 'Unknown', category: 'source' })],
      edges: [],
    };
    const node = nodeById(toFlow(graph).nodes, 'a');
    expect(node.className).toContain('bg-info-tint');
  });

  it('maps an app category with a neutral status to the default style', () => {
    const graph: Graph = {
      nodes: [makeGraphNode({ id: 'a', status: 'Unknown', ready: 'Unknown', category: 'app' })],
      edges: [],
    };
    const node = nodeById(toFlow(graph).nodes, 'a');
    expect(node.className).toContain('border-edge-emphasis');
  });

  it('maps an applier category with a neutral status to the default style', () => {
    const graph: Graph = {
      nodes: [
        makeGraphNode({ id: 'a', status: 'Progressing', ready: 'Unknown', category: 'applier' }),
      ],
      edges: [],
    };
    const node = nodeById(toFlow(graph).nodes, 'a');
    expect(node.className).toContain('border-edge-emphasis');
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
    expect(source.style).toEqual({ stroke: 'var(--graph-edge-source)' });
    expect(depends.animated).toBe(false);
    expect(depends.style).toEqual({ stroke: 'var(--graph-edge-depends)' });
    expect(manages.animated).toBe(false);
    expect(manages.style).toEqual({ stroke: 'var(--graph-edge-manages)' });
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

describe('a dependency that is not there', () => {
  it('is drawn in the failure colour, not as an ordinary applier', () => {
    const flow = toFlow({
      nodes: [
        makeGraphNode({
          id: 'a',
          name: 'infra',
          status: 'NotFound',
          ready: 'False',
          category: 'applier',
        }),
      ],
      edges: [],
    });

    expect(flow.nodes[0].className).toContain('border-error-emphasis');
  });

  it('leaves a healthy applier alone', () => {
    const flow = toFlow({
      nodes: [makeGraphNode({ id: 'a', name: 'apps', status: 'Ready', category: 'applier' })],
      edges: [],
    });

    expect(flow.nodes[0].className).toContain('border-ok-emphasis');
  });
});

describe('statusTone', () => {
  it('follows the ready state the backend reports', () => {
    expect(statusTone('True')).toBe('ok');
    expect(statusTone('False')).toBe('error');
    expect(statusTone('Unknown')).toBe('unknown');
  });
});

describe('a failure the frontend has never heard of', () => {
  it('is still drawn as a failure', () => {
    const flow = toFlow({
      nodes: [
        makeGraphNode({ id: 'a', status: 'SomeBrandNewReason', ready: 'False', category: 'app' }),
      ],
      edges: [],
    });

    expect(flow.nodes[0].className).toContain('border-error-emphasis');
  });
});

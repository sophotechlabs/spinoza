import { afterEach, describe, expect, it, vi } from 'vitest';
import dagre from '@dagrejs/dagre';
import type { Graph } from '../../src/lib/types';
import {
  busiestNamespace,
  nodeLabel,
  restyle,
  sameGraph,
  sameTopology,
  statusTone,
  toFlow,
} from '../../src/lib/graphLayout';
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

describe('comparing one poll against the last', () => {
  function graph(overrides: Partial<Graph> = {}): Graph {
    return {
      nodes: [
        makeGraphNode({ id: 'a', name: 'alpha' }),
        makeGraphNode({ id: 'b', name: 'bravo', category: 'app' }),
      ],
      edges: [makeGraphEdge({ from: 'a', to: 'b', kind: 'source' })],
      ...overrides,
    };
  }

  const single: Graph = { nodes: [makeGraphNode({ id: 'a', name: 'alpha' })], edges: [] };

  it('calls an identical payload the same graph', () => {
    expect(sameGraph(graph(), graph())).toBe(true);
    expect(sameTopology(graph(), graph())).toBe(true);
  });

  it('spots a node that changed readiness without changing the layout', () => {
    const next = graph({
      nodes: [
        makeGraphNode({ id: 'a', name: 'alpha', ready: 'False' }),
        makeGraphNode({ id: 'b', name: 'bravo', category: 'app' }),
      ],
    });

    expect(sameGraph(graph(), next)).toBe(false);
    expect(sameTopology(graph(), next)).toBe(true);
  });

  it('spots a node that changed status or namespace', () => {
    const status: Graph = {
      nodes: [makeGraphNode({ id: 'a', name: 'alpha', status: 'Failed' })],
      edges: [],
    };
    const moved: Graph = {
      nodes: [makeGraphNode({ id: 'a', name: 'alpha', namespace: 'other' })],
      edges: [],
    };

    expect(sameGraph(single, status)).toBe(false);
    expect(sameGraph(single, moved)).toBe(false);
  });

  it('spots node identity changes that affect the visible label', () => {
    const identity = makeGraphNode({
      id: 'a',
      name: 'alpha',
      kind: 'Deployment',
      group: 'apps',
      version: 'v1',
      resource: 'deployments',
    });
    const before: Graph = { nodes: [identity], edges: [] };

    expect(sameGraph(before, { nodes: [{ ...identity, kind: 'StatefulSet' }], edges: [] })).toBe(
      false,
    );
    expect(sameGraph(before, { nodes: [{ ...identity, group: 'extensions' }], edges: [] })).toBe(
      false,
    );
    expect(sameGraph(before, { nodes: [{ ...identity, version: 'v1beta1' }], edges: [] })).toBe(
      false,
    );
    expect(
      sameGraph(before, { nodes: [{ ...identity, resource: 'statefulsets' }], edges: [] }),
    ).toBe(false);
  });

  it('spots a partial-failure message that appeared', () => {
    expect(sameGraph(graph(), graph({ error: 'buckets: forbidden' }))).toBe(false);
  });

  it('spots a node added, renamed or recategorised', () => {
    expect(sameTopology(graph(), single)).toBe(false);
    expect(
      sameTopology(
        graph(),
        graph({
          nodes: [
            makeGraphNode({ id: 'a', name: 'renamed' }),
            makeGraphNode({ id: 'b', name: 'bravo', category: 'app' }),
          ],
        }),
      ),
    ).toBe(false);
    expect(
      sameTopology(
        graph(),
        graph({
          nodes: [
            makeGraphNode({ id: 'a', name: 'alpha', category: 'app' }),
            makeGraphNode({ id: 'b', name: 'bravo', category: 'app' }),
          ],
        }),
      ),
    ).toBe(false);
  });

  it('spots an edge added, removed or redirected', () => {
    expect(sameTopology(graph(), graph({ edges: [] }))).toBe(false);
    expect(
      sameTopology(
        graph(),
        graph({ edges: [makeGraphEdge({ from: 'b', to: 'a', kind: 'source' })] }),
      ),
    ).toBe(false);
    expect(
      sameTopology(
        graph(),
        graph({ edges: [makeGraphEdge({ from: 'a', to: 'c', kind: 'source' })] }),
      ),
    ).toBe(false);
    expect(
      sameTopology(
        graph(),
        graph({ edges: [makeGraphEdge({ from: 'a', to: 'b', kind: 'manages' })] }),
      ),
    ).toBe(false);
  });
});

describe('restyle', () => {
  it('keeps every position and edge while taking the new colours', () => {
    const before: Graph = {
      nodes: [makeGraphNode({ id: 'a', name: 'alpha', ready: 'True' })],
      edges: [],
    };
    const after: Graph = {
      nodes: [makeGraphNode({ id: 'a', name: 'alpha', ready: 'False' })],
      edges: [],
    };
    const laid = toFlow(before);

    const restyled = restyle(laid, after);

    expect(restyled.nodes[0].position).toEqual(laid.nodes[0].position);
    expect(restyled.edges).toBe(laid.edges);
    expect(restyled.nodes[0].className).not.toBe(laid.nodes[0].className);
    expect(restyled.nodes[0].data.node.ready).toBe('False');
  });
});

describe('nodeLabel', () => {
  it('is just the name when nothing is folded inside', () => {
    expect(nodeLabel(makeGraphNode({ name: 'api' }))).toBe('api');
  });

  it('counts what is folded inside', () => {
    expect(nodeLabel(makeGraphNode({ name: 'api', contains: 40 }))).toBe('api ×40');
  });

  it('says how much of what is folded is broken', () => {
    expect(nodeLabel(makeGraphNode({ name: 'api', contains: 40, unhealthy: 2 }))).toBe(
      'api ×40 · 2 not ready',
    );
  });

  it('adds Kind only when node names collide', () => {
    const deployment = makeGraphNode({ id: 'deployment', name: 'podinfo', kind: 'Deployment' });
    const service = makeGraphNode({ id: 'service', name: 'podinfo', kind: 'Service' });

    expect(nodeLabel(deployment, [deployment, service])).toBe('podinfo · Deployment');
    expect(nodeLabel(service, [deployment, service])).toBe('podinfo · Service');
  });

  it('adds namespace when names and Kinds still collide', () => {
    const production = makeGraphNode({ id: 'production', name: 'podinfo', namespace: 'prod' });
    const staging = makeGraphNode({ id: 'staging', name: 'podinfo', namespace: 'staging' });

    expect(nodeLabel(production, [production, staging])).toBe('podinfo · GitRepository · prod');
    expect(nodeLabel(staging, [production, staging])).toBe('podinfo · GitRepository · staging');
  });

  it('adds scope and API identity only while a collision remains', () => {
    const stable = makeGraphNode({
      id: 'stable',
      name: 'events',
      kind: 'Event',
      group: '',
      version: 'v1',
      namespace: 'prod',
    });
    const newer = makeGraphNode({
      id: 'newer',
      name: 'events',
      kind: 'Event',
      group: 'events.k8s.io',
      version: 'v1',
      namespace: 'prod',
    });

    expect(nodeLabel(stable, [stable, newer])).toBe('events · Event · prod · core/v1');
    expect(nodeLabel(newer, [stable, newer])).toBe('events · Event · prod · events.k8s.io/v1');
  });
});

describe('busiestNamespace', () => {
  it('names the namespace holding the most', () => {
    const graph: Graph = {
      nodes: [
        makeGraphNode({ id: 'a', namespace: 'prod' }),
        makeGraphNode({ id: 'b', namespace: 'prod' }),
        makeGraphNode({ id: 'c', namespace: 'staging' }),
      ],
      edges: [],
    };

    expect(busiestNamespace(graph)).toBe('prod');
  });

  it('names nothing when nothing is in a namespace', () => {
    const graph: Graph = {
      nodes: [makeGraphNode({ id: 'a', namespace: '' })],
      edges: [],
    };

    expect(busiestNamespace(graph)).toBe('');
  });
});

describe('a fold that changes size', () => {
  function one(overrides: Partial<Graph['nodes'][number]>): Graph {
    return { nodes: [makeGraphNode({ id: 'a', name: 'alpha', ...overrides })], edges: [] };
  }

  it('is a different graph when the count moves', () => {
    expect(sameGraph(one({ contains: 3 }), one({ contains: 4 }))).toBe(false);
  });

  it('is a different graph when what is broken inside moves', () => {
    expect(sameGraph(one({ contains: 3 }), one({ contains: 3, unhealthy: 1 }))).toBe(false);
  });

  it('keeps the same layout, so the nodes do not jump', () => {
    expect(sameTopology(one({ contains: 3 }), one({ contains: 4 }))).toBe(true);
  });
});

describe('flow nodes carry the size the layout assumed', () => {
  const graph = {
    nodes: [
      makeGraphNode({ id: 'a', name: 'a' }),
      makeGraphNode({ id: 'b', name: 'b' }),
      makeGraphNode({ id: 'c', name: 'c' }),
    ],
    edges: [makeGraphEdge({ from: 'a', to: 'b' }), makeGraphEdge({ from: 'b', to: 'c' })],
    error: undefined,
  };

  it('gives every node an explicit width and height', () => {
    const flow = toFlow(graph);

    expect(flow.nodes).toHaveLength(3);
    for (const node of flow.nodes) {
      expect(node.width).toBeGreaterThan(0);
      expect(node.height).toBeGreaterThan(0);
    }
  });

  it('sizes every node the same, so dagre and the canvas agree', () => {
    const flow = toFlow(graph);
    const widths = new Set(flow.nodes.map((node) => node.width));
    const heights = new Set(flow.nodes.map((node) => node.height));

    expect(widths.size).toBe(1);
    expect(heights.size).toBe(1);
  });

  it('keeps the size when only styling changes', () => {
    const flow = toFlow(graph);
    const restyled = restyle(flow, graph);

    for (const node of restyled.nodes) {
      expect(node.width).toBe(flow.nodes[0].width);
      expect(node.height).toBe(flow.nodes[0].height);
    }
  });

  it('ranks a connected graph rather than laying it out in one row', () => {
    const flow = toFlow(graph);
    const rows = new Set(flow.nodes.map((node) => node.position.y));

    expect(rows.size).toBeGreaterThan(1);
  });

  it('hands every edge through to the canvas', () => {
    const flow = toFlow(graph);

    expect(flow.edges).toHaveLength(2);
    const ids = new Set(flow.nodes.map((node) => node.id));
    for (const edge of flow.edges) {
      expect(ids.has(edge.source)).toBe(true);
      expect(ids.has(edge.target)).toBe(true);
    }
  });
});

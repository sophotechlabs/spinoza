import { describe, expect, it } from 'vitest';
import type { TrafficEdge, TrafficGraph } from '../../src/lib/types';
import {
  EDGE_DROP_STROKE,
  EDGE_FLOW_STROKE,
  edgeLabel,
  nodeLabel,
  perSecond,
  restyleTraffic,
  sameTraffic,
  sameTrafficShape,
  toTrafficFlow,
  MAX_EDGES,
} from '../../src/lib/trafficLayout';

function edge(over: Partial<TrafficEdge> = {}): TrafficEdge {
  return { from: 'apps/web', to: 'apps/api', rate: 1, dropped: 0, ...over };
}

function graph(over: Partial<TrafficGraph> = {}): TrafficGraph {
  return {
    source: 'Cilium Hubble',
    nodes: [
      { id: 'apps/web', namespace: 'apps', workload: 'web' },
      { id: 'apps/api', namespace: 'apps', workload: 'api' },
    ],
    edges: [edge()],
    ...over,
  };
}

describe('perSecond', () => {
  it('drops the decimals once the rate is in the hundreds', () => {
    expect(perSecond(138.65862)).toBe('139');
  });

  it('keeps one decimal in the tens', () => {
    expect(perSecond(12.55)).toBe('12.6');
  });

  it('keeps two decimals for a trickle', () => {
    expect(perSecond(0.062)).toBe('0.06');
  });
});

describe('edgeLabel', () => {
  it('names the flow rate alone when nothing is dropped', () => {
    expect(edgeLabel(edge({ rate: 9 }))).toBe('9.00/s');
  });

  it('names the dropped rate too when there is one', () => {
    expect(edgeLabel(edge({ rate: 9, dropped: 0.5 }))).toBe('9.00/s · 0.50 dropped');
  });
});

describe('nodeLabel', () => {
  it('names a workload by its namespace and name', () => {
    expect(nodeLabel({ id: 'apps/web', namespace: 'apps', workload: 'web' })).toBe('apps/web');
  });

  it('names a folded district by its namespace alone', () => {
    expect(nodeLabel({ id: 'apps', namespace: 'apps', workload: '' })).toBe('apps');
  });
});

describe('toTrafficFlow', () => {
  it('lays every workload out and labels the edge', () => {
    const flow = toTrafficFlow(graph());

    expect(flow.nodes.map((node) => node.id)).toEqual(['apps/web', 'apps/api']);
    expect(flow.nodes[0].data.label).toBe('apps/web');
    expect(nodeLabel(flow.nodes[0].data.node)).toBe('apps/web');
    expect(flow.nodes[0].position.x).toBeTypeOf('number');
    expect(flow.edges).toHaveLength(1);
    expect(flow.edges[0].id).toBe('apps/web->apps/api');
    expect(flow.edges[0].label).toBe('1.00/s');
  });

  it('colours a clean edge and a dropping edge differently', () => {
    const clean = toTrafficFlow(graph());
    const dropping = toTrafficFlow(graph({ edges: [edge({ dropped: 2 })] }));

    expect(clean.edges[0].style?.stroke).toBe(EDGE_FLOW_STROKE);
    expect(dropping.edges[0].style?.stroke).toBe(EDGE_DROP_STROKE);
  });
});

describe('sameTrafficShape', () => {
  it('ignores a rate that moved', () => {
    expect(sameTrafficShape(graph(), graph({ edges: [edge({ rate: 900 })] }))).toBe(true);
  });

  it('ignores an error that appeared', () => {
    expect(sameTrafficShape(graph(), graph({ error: 'gone' }))).toBe(true);
  });

  it('sees a workload appear', () => {
    const more = graph({
      nodes: [...graph().nodes, { id: 'apps/beat', namespace: 'apps', workload: 'beat' }],
    });
    expect(sameTrafficShape(graph(), more)).toBe(false);
  });

  it('sees a workload swapped for another', () => {
    const other = graph({
      nodes: [
        { id: 'apps/web', namespace: 'apps', workload: 'web' },
        { id: 'apps/store', namespace: 'apps', workload: 'store' },
      ],
    });
    expect(sameTrafficShape(graph(), other)).toBe(false);
  });

  it('sees an edge appear', () => {
    const more = graph({ edges: [edge(), edge({ from: 'apps/api', to: 'apps/web' })] });
    expect(sameTrafficShape(graph(), more)).toBe(false);
  });

  it('sees each end of an edge move', () => {
    expect(sameTrafficShape(graph(), graph({ edges: [edge({ from: 'apps/beat' })] }))).toBe(false);
    expect(sameTrafficShape(graph(), graph({ edges: [edge({ to: 'apps/beat' })] }))).toBe(false);
  });
});

describe('restyleTraffic', () => {
  it('keeps the laid-out workloads and rebuilds the edges', () => {
    const flow = toTrafficFlow(graph());
    const moved = graph({ edges: [edge({ rate: 500, dropped: 2 })] });

    const next = restyleTraffic(flow, moved);

    expect(next.nodes).toBe(flow.nodes);
    expect(next.edges[0].label).toBe('500/s · 2.00 dropped');
    expect(next.edges[0].style?.stroke).toBe(EDGE_DROP_STROKE);
  });
});

describe('sameTraffic', () => {
  it('sees an unchanged graph', () => {
    expect(sameTraffic(graph(), graph())).toBe(true);
  });

  it('sees a changed error', () => {
    expect(sameTraffic(graph(), graph({ error: 'gone' }))).toBe(false);
  });

  it('sees a workload appear', () => {
    const more = graph({
      nodes: [...graph().nodes, { id: 'apps/beat', namespace: 'apps', workload: 'beat' }],
    });
    expect(sameTraffic(graph(), more)).toBe(false);
  });

  it('sees a workload swapped for another', () => {
    const other = graph({
      nodes: [
        { id: 'apps/web', namespace: 'apps', workload: 'web' },
        { id: 'apps/store', namespace: 'apps', workload: 'store' },
      ],
    });
    expect(sameTraffic(graph(), other)).toBe(false);
  });

  it('sees an edge appear', () => {
    const more = graph({ edges: [edge(), edge({ from: 'apps/api', to: 'apps/web' })] });
    expect(sameTraffic(graph(), more)).toBe(false);
  });

  it('sees each end of an edge move', () => {
    expect(sameTraffic(graph(), graph({ edges: [edge({ from: 'apps/beat' })] }))).toBe(false);
    expect(sameTraffic(graph(), graph({ edges: [edge({ to: 'apps/beat' })] }))).toBe(false);
  });

  it('sees the rate and the dropped rate move', () => {
    expect(sameTraffic(graph(), graph({ edges: [edge({ rate: 2 })] }))).toBe(false);
    expect(sameTraffic(graph(), graph({ edges: [edge({ dropped: 1 })] }))).toBe(false);
  });
});

describe('a dense graph', () => {
  function edgesOf(count: number): TrafficEdge[] {
    return Array.from({ length: count }, (_, at) => ({
      from: `ns/from-${String(at)}`,
      to: `ns/to-${String(at)}`,
      rate: at,
      dropped: 0,
    }));
  }

  function graphOf(count: number): TrafficGraph {
    const edges = edgesOf(count);
    const names = new Set<string>();
    for (const edge of edges) {
      names.add(edge.from);
      names.add(edge.to);
    }
    return {
      source: 'Cilium Hubble',
      nodes: [...names].map((id) => ({ id, namespace: 'ns', workload: id })),
      edges,
    };
  }

  it('draws every edge while the graph is small enough to animate', () => {
    const flow = toTrafficFlow(graphOf(10));

    expect(flow.edges).toHaveLength(10);
    expect(flow.edges[0].animated).toBe(true);
    expect(flow.edges[0].label).not.toBeUndefined();
  });

  it('keeps only the busiest edges past the cap', () => {
    const flow = toTrafficFlow(graphOf(MAX_EDGES + 60));

    expect(flow.edges).toHaveLength(MAX_EDGES);
    const ids = new Set(flow.edges.map((edge) => edge.id));
    expect(ids.has('ns/from-0->ns/to-0')).toBe(false);
    expect(ids.has(`ns/from-${String(MAX_EDGES + 59)}->ns/to-${String(MAX_EDGES + 59)}`)).toBe(
      true,
    );
  });

  it('stops animating once it is dense but keeps the rate on every edge', () => {
    const flow = toTrafficFlow(graphOf(MAX_EDGES + 1));

    for (const edge of flow.edges) {
      expect(edge.animated).toBe(false);
      expect(edge.label).not.toBeUndefined();
    }
  });
});

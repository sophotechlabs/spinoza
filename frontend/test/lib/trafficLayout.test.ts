import { describe, expect, it } from 'vitest';
import type { TrafficEdge, TrafficGraph } from '../../src/lib/types';
import {
  EDGE_DROP_STROKE,
  EDGE_FLOW_STROKE,
  edgeLabel,
  perSecond,
  restyleTraffic,
  sameTraffic,
  sameTrafficShape,
  toTrafficFlow,
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

describe('toTrafficFlow', () => {
  it('lays every workload out and labels the edge', () => {
    const flow = toTrafficFlow(graph());

    expect(flow.nodes.map((node) => node.id)).toEqual(['apps/web', 'apps/api']);
    expect(flow.nodes[0].data.label).toBe('apps/web');
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

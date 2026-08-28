import { useState } from 'react';
import { Background, Controls, ReactFlow } from '@xyflow/react';
import type { TrafficGraph } from '../lib/types';
import { fetchTrafficGraph } from '../lib/traffic';
import { usePoll } from '../lib/usePoll';
import {
  EDGE_DROP_STROKE,
  EDGE_FLOW_STROKE,
  restyleTraffic,
  sameTraffic,
  sameTrafficShape,
  toTrafficFlow,
} from '../lib/trafficLayout';
import type { TrafficFlow } from '../lib/trafficLayout';
import { useResolvedTheme } from '../store/theme';
import LoadFailure from './LoadFailure';
import LoadWarning from './LoadWarning';
import Loading from './Loading';
import StaleBanner from './StaleBanner';

const POLL_INTERVAL_MS = 5000;
const MAX_NODES = 400;

const WHAT = 'The traffic graph';
const EMPTY = 'No workload-to-workload traffic in the last five minutes.';

const LEGEND = [
  { stroke: EDGE_FLOW_STROKE, label: 'Forwarded flows per second' },
  { stroke: EDGE_DROP_STROKE, label: 'Some flows dropped' },
];

interface Laid {
  graph: TrafficGraph;
  flow: TrafficFlow;
}

function layOut(current: Laid | null, graph: TrafficGraph): Laid {
  if (current === null) {
    return { graph, flow: toTrafficFlow(graph) };
  }
  if (sameTraffic(current.graph, graph)) {
    return current;
  }
  if (sameTrafficShape(current.graph, graph)) {
    return { graph, flow: restyleTraffic(current.flow, graph) };
  }
  return { graph, flow: toTrafficFlow(graph) };
}

export default function Traffic() {
  const resolvedTheme = useResolvedTheme();
  const { data, error, reload } = usePoll(fetchTrafficGraph, {
    intervalMs: POLL_INTERVAL_MS,
    fallback: 'traffic graph request failed',
  });
  const [laid, setLaid] = useState<Laid | null>(null);

  let partial: string | null = null;
  let overLimit: number | null = null;
  if (data !== null) {
    partial = data.error ?? null;
    if (data.nodes.length > MAX_NODES) {
      overLimit = data.nodes.length;
    }
  }

  if (data === null && laid !== null) {
    setLaid(null);
  }

  if (data !== null && overLimit === null) {
    const next = layOut(laid, data);
    if (next !== laid) {
      setLaid(next);
    }
  }

  if (overLimit !== null) {
    return (
      <div className="flex h-full items-center justify-center px-4 text-center text-xs text-fg-muted">
        {WHAT} has {overLimit} workloads, too many to render.
      </div>
    );
  }

  if (laid === null) {
    if (error !== null) {
      return (
        <div className="flex h-full items-center justify-center text-xs text-error">{error}</div>
      );
    }
    return <Loading what="the traffic graph" />;
  }

  if (laid.flow.nodes.length === 0) {
    if (partial !== null) {
      return <LoadFailure what={WHAT} message={partial} />;
    }
    return (
      <div className="flex h-full items-center justify-center text-xs text-fg-muted">{EMPTY}</div>
    );
  }

  return (
    <div className="flex h-full min-h-0 w-full flex-col">
      {error !== null && <StaleBanner what={WHAT} message={error} onRetry={reload} />}
      {partial !== null && <LoadWarning message={partial} />}
      <div className="relative min-h-0 w-full flex-1">
        <ReactFlow
          nodes={laid.flow.nodes}
          edges={laid.flow.edges}
          colorMode={resolvedTheme.base}
          nodesDraggable={false}
          nodesConnectable={false}
          elementsSelectable={false}
          onlyRenderVisibleElements
          fitView
        >
          <Background />
          <Controls />
        </ReactFlow>
        <div className="pointer-events-none absolute top-2 right-2 z-10 rounded border border-edge bg-surface-raised/90 px-2 py-1.5 text-[11px] text-fg-soft">
          <div className="mb-1 font-semibold text-fg">{laid.graph.source}</div>
          {LEGEND.map((item) => (
            <div key={item.label} className="flex items-center gap-1.5">
              <span className="h-0.5 w-2.5 rounded" style={{ backgroundColor: item.stroke }} />
              {item.label}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

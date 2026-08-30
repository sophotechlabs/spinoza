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
import type { TrafficFlow, TrafficFlowNode } from '../lib/trafficLayout';
import { MAX_NODES, useLaidOut } from '../lib/graphState';
import type { Laid } from '../lib/graphState';
import GraphShell from './GraphShell';

const POLL_INTERVAL_MS = 5000;

const WHAT = 'The traffic graph';
const EMPTY = 'No workload-to-workload traffic in the last five minutes.';

const LEGEND = [
  { stroke: EDGE_FLOW_STROKE, label: 'Forwarded flows per second' },
  { stroke: EDGE_DROP_STROKE, label: 'Some flows dropped' },
];

function foldedNote(graph: TrafficGraph): string | null {
  if (graph.folded !== true) {
    return null;
  }
  const workloads = graph.workloads ?? 0;
  return `Folded to namespaces: ${String(workloads)} workloads is past the ${String(MAX_NODES)} this graph draws.`;
}

function layOut(
  current: Laid<TrafficGraph, TrafficFlow> | null,
  graph: TrafficGraph,
): Laid<TrafficGraph, TrafficFlow> {
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
  const { data, error, reload } = usePoll(fetchTrafficGraph, {
    intervalMs: POLL_INTERVAL_MS,
    fallback: 'traffic graph request failed',
  });
  const { laid, partial, overLimit } = useLaidOut<TrafficGraph, TrafficFlow>(data, layOut);

  let tooMany = null;
  if (overLimit !== null) {
    tooMany = (
      <>
        {WHAT} has {overLimit} workloads, too many to render.
      </>
    );
  }

  let flow: TrafficFlow | null = null;
  let banner = null;
  let source = '';
  if (laid !== null) {
    flow = laid.flow;
    source = laid.graph.source;
    const note = foldedNote(laid.graph);
    if (note !== null) {
      banner = (
        <div
          role="status"
          className="shrink-0 border-b border-edge bg-surface-raised px-3 py-1.5 text-xs text-fg-muted"
        >
          {note}
        </div>
      );
    }
  }

  return (
    <GraphShell<TrafficFlowNode>
      what={WHAT}
      loading="the traffic graph"
      empty={EMPTY}
      flow={flow}
      error={error}
      partial={partial}
      overLimit={tooMany}
      onRetry={reload}
      banner={banner}
      legend={
        <>
          <div className="mb-1 font-semibold text-fg">{source}</div>
          {LEGEND.map((item) => (
            <div key={item.label} className="flex items-center gap-1.5">
              <span className="h-0.5 w-2.5 rounded" style={{ backgroundColor: item.stroke }} />
              {item.label}
            </div>
          ))}
        </>
      }
    />
  );
}

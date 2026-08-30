import { useCallback, useEffect } from 'react';
import { useReactFlow } from '@xyflow/react';
import type { NodeMouseHandler } from '@xyflow/react';
import type { Graph, GraphEdgeKind, GraphNode } from '../lib/types';
import {
  EDGE_STROKES,
  busiestNamespace,
  restyle,
  sameGraph,
  sameTopology,
  toFlow,
} from '../lib/graphLayout';
import type { GitopsFlow, GitopsFlowNode } from '../lib/graphLayout';
import { useLaidOut } from '../lib/graphState';
import type { Laid } from '../lib/graphState';
import GraphShell from './GraphShell';

const ALL_EDGE_KINDS: GraphEdgeKind[] = ['source', 'dependsOn', 'manages'];

function shapeOf(flow: GitopsFlow): string {
  return `${flow.nodes.length}:${flow.edges.length}:${flow.nodes[0]?.id ?? ''}`;
}

function Refit({ shape }: { shape: string }) {
  const flow = useReactFlow();
  useEffect(() => {
    void flow.fitView();
  }, [flow, shape]);
  return null;
}

interface GraphCanvasProps {
  what: string;
  empty: string;
  data: Graph | null;
  error: string | null;
  edgeKinds?: GraphEdgeKind[];
  onRetry: () => void;
  onSelect?: (node: GraphNode) => void;
}

interface LegendItem {
  swatch: string;
  label: string;
}

const SOURCE_LEGEND: LegendItem = {
  swatch: 'border-info-line bg-info-tint',
  label: 'Source, not ready yet',
};

const LEGEND: LegendItem[] = [
  { swatch: 'border-ok-emphasis bg-ok-tint', label: 'Ready' },
  { swatch: 'border-error-emphasis bg-error-tint', label: 'Not ready or missing' },
  { swatch: 'border-edge-emphasis bg-surface-active', label: 'Unknown' },
];

function narrowTo(graph: Graph): string {
  const busiest = busiestNamespace(graph);
  if (busiest === '') {
    return 'Narrow the view down.';
  }
  return `Pick a namespace — ${busiest} is the biggest.`;
}

function legendFor(graph: Graph | null): LegendItem[] {
  if (graph === null) {
    return LEGEND;
  }
  if (graph.nodes.some((node) => node.category === 'source')) {
    return [...LEGEND, SOURCE_LEGEND];
  }
  return LEGEND;
}

const EDGE_LABELS: Record<GraphEdgeKind, string> = {
  source: 'Source',
  dependsOn: 'Depends on',
  manages: 'Manages',
  owns: 'Owns',
  routes: 'Routes to',
  configures: 'Configures',
  scales: 'Scales',
};

function layOut(current: Laid<Graph, GitopsFlow> | null, graph: Graph): Laid<Graph, GitopsFlow> {
  if (current === null) {
    return { graph, flow: toFlow(graph) };
  }
  if (sameGraph(current.graph, graph)) {
    return current;
  }
  if (sameTopology(current.graph, graph)) {
    return { graph, flow: restyle(current.flow, graph) };
  }
  return { graph, flow: toFlow(graph) };
}

export default function GraphCanvas({
  what,
  empty,
  data,
  error,
  edgeKinds = ALL_EDGE_KINDS,
  onRetry,
  onSelect,
}: GraphCanvasProps) {
  const { laid, partial, overLimit } = useLaidOut<Graph, GitopsFlow>(data, layOut);

  const handleNodeClick = useCallback<NodeMouseHandler<GitopsFlowNode>>(
    (_event, node) => {
      if (onSelect) {
        onSelect(node.data.node);
      }
    },
    [onSelect],
  );

  let tooMany = null;
  if (overLimit !== null && data !== null) {
    tooMany = (
      <>
        {what} has {overLimit} nodes, too many to render. {narrowTo(data)}
      </>
    );
  }

  let flow: GitopsFlow | null = null;
  if (laid !== null) {
    flow = laid.flow;
  }

  let refit = null;
  if (flow !== null) {
    refit = <Refit shape={shapeOf(flow)} />;
  }

  return (
    <GraphShell<GitopsFlowNode>
      what={what}
      loading="graph"
      empty={empty}
      flow={flow}
      error={error}
      partial={partial}
      overLimit={tooMany}
      onRetry={onRetry}
      onNodeClick={handleNodeClick}
      legend={
        <>
          {legendFor(laid?.graph ?? null).map((item) => (
            <div key={item.label} className="flex items-center gap-1.5">
              <span className={`h-2.5 w-2.5 rounded border ${item.swatch}`} />
              {item.label}
            </div>
          ))}
          {edgeKinds.map((kind) => (
            <div key={kind} className="flex items-center gap-1.5">
              <span
                className="h-0.5 w-2.5 rounded"
                style={{ backgroundColor: EDGE_STROKES[kind] }}
              />
              {EDGE_LABELS[kind]}
            </div>
          ))}
        </>
      }
    >
      {refit}
    </GraphShell>
  );
}

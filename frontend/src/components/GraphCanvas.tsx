import { useCallback, useState } from 'react';
import { Background, Controls, ReactFlow } from '@xyflow/react';
import type { NodeMouseHandler } from '@xyflow/react';
import type { Graph, GraphEdgeKind, GraphNode } from '../lib/types';
import {
  EDGE_DEPENDS_STROKE,
  EDGE_MANAGES_STROKE,
  EDGE_SOURCE_STROKE,
  restyle,
  sameGraph,
  sameTopology,
  toFlow,
} from '../lib/graphLayout';
import type { GitopsFlow, GitopsFlowNode } from '../lib/graphLayout';
import LoadWarning from './LoadWarning';
import LoadFailure from './LoadFailure';
import StaleBanner from './StaleBanner';
import { useResolvedTheme } from '../store/theme';

const MAX_NODES = 400;

const ALL_EDGE_KINDS: GraphEdgeKind[] = ['source', 'dependsOn', 'manages'];

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

function legendFor(graph: Graph): LegendItem[] {
  if (graph.nodes.some((node) => node.category === 'source')) {
    return [...LEGEND, SOURCE_LEGEND];
  }
  return LEGEND;
}

const EDGE_LABELS: Record<GraphEdgeKind, string> = {
  source: 'Source',
  dependsOn: 'Depends on',
  manages: 'Manages',
};

const EDGE_STROKES: Record<GraphEdgeKind, string> = {
  source: EDGE_SOURCE_STROKE,
  dependsOn: EDGE_DEPENDS_STROKE,
  manages: EDGE_MANAGES_STROKE,
};

interface Laid {
  graph: Graph;
  flow: GitopsFlow;
}

function layOut(current: Laid | null, graph: Graph): Laid {
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
  const resolvedTheme = useResolvedTheme();
  const [laid, setLaid] = useState<Laid | null>(null);

  let partial: string | null = null;
  let overLimit: number | null = null;
  if (data !== null) {
    partial = data.error ?? null;
    if (data.nodes.length > MAX_NODES) {
      overLimit = data.nodes.length;
    }
  }

  if (data !== null && overLimit === null) {
    const next = layOut(laid, data);
    if (next !== laid) {
      setLaid(next);
    }
  }

  let flow: GitopsFlow | null = null;
  let legend = LEGEND;
  if (laid !== null) {
    flow = laid.flow;
    legend = legendFor(laid.graph);
  }

  const handleNodeClick = useCallback<NodeMouseHandler<GitopsFlowNode>>(
    (_event, node) => {
      if (onSelect) {
        onSelect(node.data.node);
      }
    },
    [onSelect],
  );

  if (overLimit !== null) {
    return (
      <div className="flex h-full items-center justify-center px-4 text-center text-xs text-fg-muted">
        {what} has {overLimit} nodes, too many to render.
      </div>
    );
  }

  if (flow === null) {
    if (error !== null) {
      return (
        <div className="flex h-full items-center justify-center text-xs text-error">{error}</div>
      );
    }
    return (
      <div className="flex h-full items-center justify-center text-xs text-fg-muted">
        Loading graph
      </div>
    );
  }

  if (flow.nodes.length === 0) {
    if (partial !== null) {
      return <LoadFailure what={what} message={partial} />;
    }
    return (
      <div className="flex h-full items-center justify-center text-xs text-fg-muted">{empty}</div>
    );
  }

  return (
    <div className="flex h-full min-h-0 w-full flex-col">
      {error !== null && <StaleBanner what={what} message={error} onRetry={onRetry} />}
      {partial !== null && <LoadWarning message={partial} />}
      <div className="relative min-h-0 w-full flex-1">
        <ReactFlow
          nodes={flow.nodes}
          edges={flow.edges}
          onNodeClick={handleNodeClick}
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
          {legend.map((item) => (
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
        </div>
      </div>
    </div>
  );
}

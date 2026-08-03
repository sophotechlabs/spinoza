import { useCallback, useEffect, useState } from 'react';
import { Background, Controls, ReactFlow } from '@xyflow/react';
import type { NodeMouseHandler } from '@xyflow/react';
import type { GraphNode } from '../lib/types';
import { fetchGraph } from '../lib/graph';
import { controlPlane, toFlow } from '../lib/graphLayout';
import type { GitopsFlow, GitopsFlowNode } from '../lib/graphLayout';
import LoadWarning from './LoadWarning';
import LoadFailure from './LoadFailure';

const POLL_INTERVAL_MS = 5000;
const MAX_NODES = 400;

interface GitopsGraphProps {
  onSelect?: (node: GraphNode) => void;
}

interface LegendItem {
  color: string;
  label: string;
}

const LEGEND: LegendItem[] = [
  { color: 'bg-green-500', label: 'Ready' },
  { color: 'bg-red-500', label: 'Not ready' },
  { color: 'bg-sky-500', label: 'Source' },
  { color: 'bg-neutral-500', label: 'Managed' },
];

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'gitops graph request failed';
}

export default function GitopsGraph({ onSelect }: GitopsGraphProps) {
  const [flow, setFlow] = useState<GitopsFlow | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [partial, setPartial] = useState<string | null>(null);
  const [overLimit, setOverLimit] = useState<number | null>(null);

  useEffect(() => {
    let mounted = true;
    const load = async () => {
      try {
        const graph = await fetchGraph();
        if (mounted) {
          setPartial(graph.error ?? null);
          const reduced = controlPlane(graph);
          if (reduced.nodes.length > MAX_NODES) {
            setOverLimit(reduced.nodes.length);
            setFlow(null);
          } else {
            setOverLimit(null);
            setFlow(toFlow(reduced));
          }
          setError(null);
        }
      } catch (err: unknown) {
        if (mounted) {
          setError(errorMessage(err));
        }
      }
    };
    void load();
    const timer = setInterval(() => {
      void load();
    }, POLL_INTERVAL_MS);
    return () => {
      mounted = false;
      clearInterval(timer);
    };
  }, []);

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
      <div className="flex h-full items-center justify-center px-4 text-center text-xs text-neutral-500">
        GitOps control plane has {overLimit} nodes — too many to render.
      </div>
    );
  }

  if (flow === null) {
    if (error !== null) {
      return (
        <div className="flex h-full items-center justify-center text-xs text-red-400">{error}</div>
      );
    }
    return (
      <div className="flex h-full items-center justify-center text-xs text-neutral-600">
        Loading graph…
      </div>
    );
  }

  if (flow.nodes.length === 0) {
    if (partial !== null) {
      return <LoadFailure what="The GitOps graph" message={partial} />;
    }
    return (
      <div className="flex h-full items-center justify-center text-xs text-neutral-600">
        No GitOps resources found.
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 w-full flex-col">
      {partial !== null && <LoadWarning message={partial} />}
      <div className="relative min-h-0 w-full flex-1">
        <ReactFlow
          nodes={flow.nodes}
          edges={flow.edges}
          onNodeClick={handleNodeClick}
          onlyRenderVisibleElements
          fitView
        >
          <Background />
          <Controls />
        </ReactFlow>
        <div className="pointer-events-none absolute top-2 right-2 z-10 rounded border border-neutral-800 bg-neutral-900/90 px-2 py-1.5 text-[11px] text-neutral-300">
          {LEGEND.map((item) => (
            <div key={item.label} className="flex items-center gap-1.5">
              <span className={`h-2 w-2 rounded-full ${item.color}`} />
              {item.label}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

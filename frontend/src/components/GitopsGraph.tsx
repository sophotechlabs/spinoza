import { useCallback, useEffect, useState } from 'react';
import { Background, Controls, ReactFlow } from '@xyflow/react';
import type { NodeMouseHandler } from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import type { GraphNode } from '../lib/types';
import { fetchGraph } from '../lib/graph';
import { toFlow } from '../lib/graphLayout';
import type { GitopsFlow, GitopsFlowNode } from '../lib/graphLayout';

const POLL_INTERVAL_MS = 5000;

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

  useEffect(() => {
    let mounted = true;
    const load = async () => {
      try {
        const graph = await fetchGraph();
        if (mounted) {
          setFlow(toFlow(graph));
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
    return (
      <div className="flex h-full items-center justify-center text-xs text-neutral-600">
        No GitOps resources found.
      </div>
    );
  }

  return (
    <div className="relative h-full w-full">
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
  );
}

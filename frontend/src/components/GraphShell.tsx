import type { ReactNode } from 'react';
import { Background, Controls, ReactFlow } from '@xyflow/react';
import type { Edge, Node, NodeMouseHandler } from '@xyflow/react';
import LoadFailure from './LoadFailure';
import LoadWarning from './LoadWarning';
import StaleBanner from './StaleBanner';
import Loading from './Loading';
import { useResolvedTheme } from '../store/theme';

interface Flow<N extends Node> {
  nodes: N[];
  edges: Edge[];
}

interface GraphShellProps<N extends Node> {
  what: string;
  loading: string;
  empty: string;
  flow: Flow<N> | null;
  error: string | null;
  partial: string | null;
  overLimit: ReactNode | null;
  onRetry: () => void;
  onNodeClick?: NodeMouseHandler<N>;
  banner?: ReactNode;
  legend: ReactNode;
  children?: ReactNode;
}

export default function GraphShell<N extends Node>({
  what,
  loading,
  empty,
  flow,
  error,
  partial,
  overLimit,
  onRetry,
  onNodeClick,
  banner,
  legend,
  children,
}: GraphShellProps<N>) {
  const resolvedTheme = useResolvedTheme();

  if (overLimit !== null) {
    return (
      <div className="flex h-full items-center justify-center px-4 text-center text-xs text-fg-muted">
        {overLimit}
      </div>
    );
  }

  if (flow === null) {
    if (error !== null) {
      return (
        <div className="flex h-full items-center justify-center text-xs text-error">{error}</div>
      );
    }
    return <Loading what={loading} />;
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
      {banner}
      <div className="relative min-h-0 w-full flex-1">
        <ReactFlow
          nodes={flow.nodes}
          edges={flow.edges}
          onNodeClick={onNodeClick}
          colorMode={resolvedTheme.base}
          nodesDraggable={false}
          nodesConnectable={false}
          elementsSelectable={false}
          onlyRenderVisibleElements
          fitView
        >
          {children}
          <Background />
          <Controls />
        </ReactFlow>
        <div className="pointer-events-none absolute top-2 right-2 z-10 rounded border border-edge bg-surface-raised/90 px-2 py-1.5 text-[11px] text-fg-soft">
          {legend}
        </div>
      </div>
    </div>
  );
}

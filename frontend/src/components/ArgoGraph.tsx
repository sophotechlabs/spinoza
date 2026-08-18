import { useMemo } from 'react';
import type { Graph, GraphEdgeKind, GraphNode } from '../lib/types';
import { graphOf, useArgo } from '../lib/argocd';
import GraphCanvas from './GraphCanvas';

const EDGE_KINDS: GraphEdgeKind[] = ['manages'];

interface ArgoGraphProps {
  onSelect?: (node: GraphNode) => void;
}

export default function ArgoGraph({ onSelect }: ArgoGraphProps) {
  const { data, error, reload } = useArgo();
  const graph = useMemo<Graph | null>(() => {
    if (data === null) {
      return null;
    }
    return graphOf(data);
  }, [data]);

  return (
    <GraphCanvas
      what="The Argo CD graph"
      empty="No Argo CD applications on this cluster."
      data={graph}
      error={error}
      edgeKinds={EDGE_KINDS}
      onRetry={reload}
      onSelect={onSelect}
    />
  );
}

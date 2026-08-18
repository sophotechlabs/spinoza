import type { GraphNode } from '../lib/types';
import { fetchGraph } from '../lib/graph';
import { usePoll } from '../lib/usePoll';
import GraphCanvas from './GraphCanvas';

const POLL_INTERVAL_MS = 5000;

interface GitopsGraphProps {
  onSelect?: (node: GraphNode) => void;
}

export default function GitopsGraph({ onSelect }: GitopsGraphProps) {
  const { data, error, reload } = usePoll(fetchGraph, {
    intervalMs: POLL_INTERVAL_MS,
    fallback: 'gitops graph request failed',
  });

  return (
    <GraphCanvas
      what="The GitOps graph"
      empty="No GitOps resources found."
      data={data}
      error={error}
      onRetry={reload}
      onSelect={onSelect}
    />
  );
}

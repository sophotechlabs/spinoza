import { useCallback, useState } from 'react';
import type { GraphEdgeKind, GraphNode, ObjectRef } from '../lib/types';
import { fetchTopology } from '../lib/topology';
import { usePoll } from '../lib/usePoll';
import { useNamespace } from '../store/namespace';
import GraphCanvas from './GraphCanvas';

const POLL_INTERVAL_MS = 5000;

const EDGE_KINDS: GraphEdgeKind[] = ['owns', 'routes', 'configures', 'scales'];

interface TopologyGraphProps {
  openedOn: ObjectRef | null;
  onSelect?: (node: GraphNode) => void;
}

export default function TopologyGraph({ openedOn, onSelect }: TopologyGraphProps) {
  const namespace = useNamespace();
  const [root, setRoot] = useState<ObjectRef | null>(openedOn);
  const [expanded, setExpanded] = useState<string[]>([]);
  const [scope, setScope] = useState(namespace);

  if (scope !== namespace) {
    setScope(namespace);
    setRoot(null);
    setExpanded([]);
  }

  const fetcher = useCallback(() => {
    return fetchTopology({ namespace, expanded, root });
  }, [namespace, expanded, root]);

  const { data, error, reload } = usePoll(fetcher, {
    intervalMs: POLL_INTERVAL_MS,
    fallback: 'topology request failed',
  });

  function handleNode(node: GraphNode) {
    if (expanded.includes(node.id)) {
      setExpanded(expanded.filter((id) => id !== node.id));
      return;
    }
    if (node.contains > 0) {
      setExpanded([...expanded, node.id]);
      return;
    }
    if (onSelect) {
      onSelect(node);
    }
  }

  function widen() {
    setRoot(null);
    setExpanded([]);
  }

  return (
    <div className="flex h-full min-h-0 w-full flex-col">
      {root !== null && (
        <div className="flex items-center gap-2 border-b border-edge px-3 py-1 text-xs text-fg-soft">
          <span>
            Around {root.resource}/{root.name}
          </span>
          <button
            type="button"
            className="rounded border border-edge px-1.5 py-0.5 text-fg hover:bg-surface-raised"
            onClick={widen}
          >
            Show the whole scope
          </button>
        </div>
      )}
      <div className="min-h-0 flex-1">
        <GraphCanvas
          what="The topology graph"
          empty={emptyMessage(namespace)}
          data={data}
          error={error}
          edgeKinds={EDGE_KINDS}
          onRetry={reload}
          onSelect={handleNode}
        />
      </div>
    </div>
  );
}

function emptyMessage(namespace: string): string {
  if (namespace === '') {
    return 'No workloads found in this cluster.';
  }
  return `No workloads found in ${namespace}.`;
}

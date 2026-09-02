import { useEffect, useState } from 'react';
import type { NodeShellSupport } from '../lib/types';
import { fetchNodeShellSupport } from '../lib/exec';
import { useTerminalsStore } from '../store/terminals';
import { useNodeShell } from '../store/settings';
import { revealPanel } from '../store/panels';
import { useClusterEpoch } from '../store/cluster';

interface NodeShellButtonProps {
  node: string;
}

const BUTTON =
  'rounded border border-edge-strong px-2 py-1 text-fg hover:bg-surface-active disabled:cursor-not-allowed disabled:text-fg-faint';

function why(support: NodeShellSupport | null): string {
  if (support === null) {
    return 'Checking whether a node shell can be opened';
  }
  if (support.reason !== undefined && support.reason !== '') {
    return support.reason;
  }
  return `Opens a root shell on ${support.node} by running a privileged ${support.image} pod in ${support.namespace}`;
}

export default function NodeShellButton({ node }: NodeShellButtonProps) {
  const epoch = useClusterEpoch();
  const [support, setSupport] = useState<NodeShellSupport | null>(null);
  const [askedFor, setAskedFor] = useState('');
  const openNode = useTerminalsStore((state) => state.openNode);
  const turnedOn = useNodeShell();
  const targetKey = `${epoch}|${node}|${String(turnedOn)}`;

  useEffect(() => {
    let live = true;
    setSupport(null);
    fetchNodeShellSupport(node)
      .then((found) => {
        if (live) {
          setSupport(found);
          setAskedFor(targetKey);
        }
      })
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [node, turnedOn, targetKey, epoch]);

  let visibleSupport = support;
  if (askedFor !== targetKey) {
    visibleSupport = null;
  }
  const ready = visibleSupport !== null && visibleSupport.enabled && visibleSupport.allowed;

  return (
    <span title={why(visibleSupport)}>
      <button
        type="button"
        disabled={!ready}
        onClick={() => {
          openNode(node);
          revealPanel('terminal');
        }}
        className={BUTTON}
      >
        Node shell
      </button>
    </span>
  );
}

import { useState } from 'react';
import { activateCluster, closeCluster, clusterFailure } from '../lib/clusters';
import { forgetTab } from '../lib/tabs';
import { useActiveCluster, useClustersStore, useTabs } from '../store/clusters';
import type { Tab } from '../store/clusters';
import { useClusterHealthStore } from '../store/clusterHealth';
import { notifyError } from '../store/toasts';

interface ClusterStripProps {
  onShown: () => void;
}

const TAB =
  'flex items-center gap-1.5 rounded-t border-x border-t px-2 py-1 whitespace-nowrap max-w-56';

function tabClass(active: boolean): string {
  if (active) {
    return `${TAB} border-edge-strong bg-surface-raised text-fg-strong`;
  }
  return `${TAB} border-edge bg-surface text-fg-soft hover:bg-surface-active`;
}

function dotClass(reachable: boolean): string {
  if (reachable) {
    return 'h-1.5 w-1.5 shrink-0 rounded-full bg-ok-solid';
  }
  return 'h-1.5 w-1.5 shrink-0 rounded-full bg-error-solid';
}

function dotLabel(reachable: boolean): string {
  if (reachable) {
    return 'answering';
  }
  return 'not answering';
}

export default function ClusterStrip({ onShown }: ClusterStripProps) {
  const tabs = useTabs();
  const active = useActiveCluster();
  const health = useClusterHealthStore((state) => state.byCluster);
  const [busy, setBusy] = useState(false);

  if (tabs.length < 2) {
    return null;
  }

  async function show(tab: Tab) {
    if (tab.id === active) {
      return;
    }
    useClustersStore.getState().focus(tab.id);
    onShown();
    try {
      await activateCluster(tab.id);
    } catch (err: unknown) {
      notifyError(`Switching to ${tab.context}: ${clusterFailure(err, 'the request failed')}`);
    }
  }

  async function close(tab: Tab) {
    setBusy(true);
    try {
      await closeCluster(tab.id);
      forgetTab(tab.id);
      onShown();
    } catch (err: unknown) {
      notifyError(`Closing ${tab.context}: ${clusterFailure(err, 'the request failed')}`);
    } finally {
      setBusy(false);
    }
  }

  return (
    <nav
      aria-label="Open clusters"
      className="flex shrink-0 items-end gap-1 border-b border-edge bg-surface px-2 pt-1 text-xs"
    >
      {tabs.map((tab) => (
        <span key={tab.id} className={tabClass(tab.id === active)}>
          <span
            aria-label={dotLabel(health[tab.id]?.reachable ?? true)}
            title={health[tab.id]?.reason ?? ''}
            className={dotClass(health[tab.id]?.reachable ?? true)}
          />
          <button
            type="button"
            aria-current={tab.id === active}
            title={tab.id}
            onClick={() => void show(tab)}
            className="truncate"
          >
            {tab.context}
          </button>
          <button
            type="button"
            aria-label={`Close ${tab.context}`}
            title={`Close ${tab.context}`}
            disabled={busy}
            onClick={() => void close(tab)}
            className="shrink-0 px-0.5 text-fg-muted hover:text-fg disabled:opacity-50"
          >
            ×
          </button>
        </span>
      ))}
    </nav>
  );
}

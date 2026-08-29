import { useEffect, useState } from 'react';
import { activateCluster, closeCluster, clusterFailure, recolorCluster } from '../lib/clusters';
import { colorNames, colorVar } from '../lib/clusterColor';
import { attachedTo, forgetTab } from '../lib/tabs';
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

function showing(open: string, wanted: string): string {
  if (open === wanted) {
    return '';
  }
  return wanted;
}

function dotLabel(reachable: boolean): string {
  if (reachable) {
    return 'answering';
  }
  return 'not answering';
}

function swatchClass(reachable: boolean): string {
  if (reachable) {
    return 'h-2.5 w-2.5 shrink-0 rounded-sm';
  }
  return 'h-2.5 w-2.5 shrink-0 rounded-sm ring-1 ring-error-solid';
}

export default function ClusterStrip({ onShown }: ClusterStripProps) {
  const tabs = useTabs();
  const active = useActiveCluster();
  const health = useClusterHealthStore((state) => state.byCluster);
  const [busy, setBusy] = useState(false);
  const [asking, setAsking] = useState<Tab | null>(null);
  const [painting, setPainting] = useState('');

  useEffect(() => {
    if (painting === '') {
      return;
    }
    function away() {
      setPainting('');
    }
    document.addEventListener('pointerdown', away);
    return () => {
      document.removeEventListener('pointerdown', away);
    };
  }, [painting]);

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

  async function drop(tab: Tab) {
    setAsking(null);
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

  function close(tab: Tab) {
    if (attachedTo(tab.id).length > 0) {
      setAsking(tab);
      return;
    }
    void drop(tab);
  }

  async function paint(tab: Tab, color: number) {
    setPainting('');
    try {
      await recolorCluster(tab.id, color);
    } catch (err: unknown) {
      notifyError(`Recolouring ${tab.context}: ${clusterFailure(err, 'the request failed')}`);
    }
  }

  return (
    <nav
      aria-label="Open clusters"
      className="flex shrink-0 items-end gap-1 border-b border-edge bg-surface px-2 pt-1 text-xs"
    >
      {tabs.map((tab) => (
        <span key={tab.id} className={`relative ${tabClass(tab.id === active)}`}>
          <button
            type="button"
            aria-label={`${tab.context} is ${dotLabel(health[tab.id]?.reachable ?? true)}; change its colour`}
            title={health[tab.id]?.reason ?? 'Change the colour'}
            onPointerDown={(event) => {
              event.stopPropagation();
            }}
            onClick={() => {
              setPainting(showing(painting, tab.id));
            }}
            style={{ backgroundColor: colorVar(tab.color) }}
            className={swatchClass(health[tab.id]?.reachable ?? true)}
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
            onClick={() => {
              close(tab);
            }}
            className="shrink-0 px-0.5 text-fg-muted hover:text-fg disabled:opacity-50"
          >
            ×
          </button>
          {painting === tab.id && (
            <div
              role="group"
              aria-label={`Colour for ${tab.context}`}
              onPointerDown={(event) => {
                event.stopPropagation();
              }}
              className="absolute top-full left-0 z-30 mt-1 flex gap-1 rounded border border-edge-strong bg-surface-raised p-1.5 shadow"
            >
              {colorNames().map((color) => (
                <button
                  key={color}
                  type="button"
                  aria-label={`Colour ${String(color)}`}
                  onClick={() => void paint(tab, color)}
                  style={{ backgroundColor: colorVar(color) }}
                  className="h-4 w-4 rounded-sm border border-edge-strong"
                />
              ))}
            </div>
          )}
        </span>
      ))}
      {asking !== null && (
        <dialog
          open
          aria-label={`Close ${asking.context}`}
          className="fixed inset-0 z-40 m-auto w-[26rem] rounded border border-warn-line bg-surface p-0 text-fg"
        >
          <div className="border-b border-edge px-3 py-2 text-xs font-semibold tracking-wide text-warn uppercase">
            Something is still attached
          </div>
          <div className="p-3 text-xs">
            <p className="text-fg-soft">
              <span className="font-semibold text-fg-strong">{asking.context}</span> still has{' '}
              {attachedTo(asking.id).join(' and ')} open. Closing the tab ends them.
            </p>
            <div className="mt-3 flex items-center justify-end gap-2">
              <button
                type="button"
                onClick={() => {
                  setAsking(null);
                }}
                className="rounded border border-edge-strong px-2 py-1 text-fg-soft hover:bg-surface-active"
              >
                Keep it open
              </button>
              <button
                type="button"
                onClick={() => void drop(asking)}
                className="rounded border border-error-line-strong px-2 py-1 text-error-contrast hover:bg-error-tint-strong"
              >
                Close it
              </button>
            </div>
          </div>
        </dialog>
      )}
    </nav>
  );
}

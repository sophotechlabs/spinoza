import { useEffect, useRef, useState } from 'react';
import { activateCluster, closeCluster, clusterFailure } from '../lib/clusters';
import { colorVar } from '../lib/clusterColor';
import { attachedTo, forgetTab, tabWidth } from '../lib/tabs';
import { nameOf, useActiveCluster, useTabs } from '../store/clusters';
import type { Tab } from '../store/clusters';
import TabMenu from './TabMenu';
import { useClusterHealthStore } from '../store/clusterHealth';
import { notifyError } from '../store/toasts';

interface ClusterStripProps {
  onShown: () => void;
}

const TAB = 'flex shrink-0 items-center gap-1.5 rounded-t border-x border-t px-2 py-1';

function tabClass(active: boolean, open: number): string {
  const room = tabWidth(open);
  if (active) {
    return `${TAB} ${room} border-edge-strong bg-surface-raised text-fg-strong`;
  }
  return `${TAB} ${room} border-edge bg-surface text-fg-soft hover:bg-surface-active`;
}

function showing(open: string, wanted: string): string {
  if (open === wanted) {
    return '';
  }
  return wanted;
}

function inGroups(tabs: Tab[]): { name: string; tabs: Tab[] }[] {
  const runs: { name: string; tabs: Tab[] }[] = [];
  for (const tab of [...tabs].sort((a, b) => a.grouping.localeCompare(b.grouping))) {
    const last = runs.at(-1);
    if (last?.name === tab.grouping) {
      last.tabs.push(tab);
      continue;
    }
    runs.push({ name: tab.grouping, tabs: [tab] });
  }
  return runs;
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
  const working = useRef(false);
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
    if (tab.id === active || working.current) {
      return;
    }
    working.current = true;
    setBusy(true);
    try {
      await activateCluster(tab.id);
    } catch (err: unknown) {
      notifyError(`Switching to ${tab.context}: ${clusterFailure(err, 'the request failed')}`);
      return;
    } finally {
      working.current = false;
      setBusy(false);
    }
    onShown();
  }

  async function drop(tab: Tab) {
    if (working.current) {
      return;
    }
    working.current = true;
    setAsking(null);
    setBusy(true);
    try {
      await closeCluster(tab.id);
      forgetTab(tab.id);
      onShown();
    } catch (err: unknown) {
      notifyError(`Closing ${tab.context}: ${clusterFailure(err, 'the request failed')}`);
    } finally {
      working.current = false;
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

  return (
    <nav
      aria-label="Open clusters"
      className="flex shrink-0 items-end gap-1 overflow-x-auto border-b border-edge bg-surface px-2 pt-1 text-xs"
    >
      {inGroups(tabs).map((run) => (
        <span key={run.name} className="flex shrink-0 items-end gap-1">
          {run.name !== '' && (
            <span className="px-1 pb-1.5 text-[11px] tracking-wide text-fg-muted uppercase">
              {run.name}
            </span>
          )}
          {run.tabs.map((tab) => (
            <span key={tab.id} className={`relative ${tabClass(tab.id === active, tabs.length)}`}>
              <button
                type="button"
                aria-label={`${nameOf(tab)} is ${dotLabel(health[tab.id]?.reachable ?? true)}; open its settings`}
                title={health[tab.id]?.reason ?? 'Settings for this tab'}
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
                disabled={busy}
                onClick={() => void show(tab)}
                className="truncate disabled:text-fg-subtle"
              >
                {nameOf(tab)}
              </button>
              <button
                type="button"
                aria-label={`Close ${nameOf(tab)}`}
                title={`Close ${nameOf(tab)}`}
                disabled={busy}
                onClick={() => {
                  close(tab);
                }}
                className="shrink-0 px-0.5 text-fg-muted hover:text-fg disabled:opacity-50"
              >
                ×
              </button>
              {painting === tab.id && (
                <TabMenu
                  tab={tab}
                  onDone={() => {
                    setPainting('');
                  }}
                />
              )}
            </span>
          ))}
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

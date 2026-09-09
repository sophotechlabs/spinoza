import { useEffect, useRef, useState } from 'react';
import { activateCluster, closeCluster, clusterFailure } from '../lib/clusters';
import { colorVar } from '../lib/clusterColor';
import { anchorOf, attachedTo, forgetTab, tabWidth } from '../lib/tabs';
import { nameOf, useActiveCluster, useTabs } from '../store/clusters';
import type { Tab } from '../store/clusters';
import TabMenu from './TabMenu';
import ContextPicker from './ContextPicker';
import { useClusterHealthStore } from '../store/clusterHealth';
import { notifyError } from '../store/toasts';
import { useFeedDown } from '../lib/health';

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

function openMenu(tabs: Tab[], id: string): Tab | null {
  for (const tab of tabs) {
    if (tab.id === id) {
      return tab;
    }
  }
  return null;
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

function dotLabel(reachable: boolean, unknown: boolean): string {
  if (unknown) {
    return 'of unknown health';
  }
  if (reachable) {
    return 'answering';
  }
  return 'not answering';
}

function swatchClass(reachable: boolean, unknown: boolean): string {
  if (unknown) {
    return 'h-2.5 w-2.5 shrink-0 rounded-sm opacity-50 ring-1 ring-edge-strong';
  }
  if (reachable) {
    return 'h-2.5 w-2.5 shrink-0 rounded-sm';
  }
  return 'h-2.5 w-2.5 shrink-0 rounded-sm ring-1 ring-error-solid';
}

function swatchTitle(reason: string | undefined, unknown: boolean): string {
  if (unknown) {
    return 'The feed is down, so this cluster\u2019s health is not known';
  }
  return reason ?? 'Settings for this tab';
}

export default function ClusterStrip({ onShown }: ClusterStripProps) {
  const tabs = useTabs();
  const active = useActiveCluster();
  const health = useClusterHealthStore((state) => state.byCluster);
  const unknown = useFeedDown();
  const [busy, setBusy] = useState(false);
  const working = useRef(false);
  const [asking, setAsking] = useState<Tab | null>(null);
  const [painting, setPainting] = useState('');
  const [paintAt, setPaintAt] = useState(0);
  const strip = useRef<HTMLDivElement>(null);
  const painted = openMenu(tabs, painting);

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
    <div
      ref={strip}
      className="relative flex shrink-0 items-end gap-1 border-b border-edge bg-surface px-2 pt-1 text-xs"
    >
      <nav
        aria-label="Open clusters"
        className="flex min-w-0 flex-1 items-end gap-1 overflow-x-auto"
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
                  aria-label={`${nameOf(tab)} is ${dotLabel(health[tab.id]?.reachable ?? true, unknown)}; open its tab menu`}
                  title={swatchTitle(health[tab.id]?.reason, unknown)}
                  onPointerDown={(event) => {
                    event.stopPropagation();
                  }}
                  onClick={(event) => {
                    setPaintAt(anchorOf(event.currentTarget, strip.current));
                    setPainting(showing(painting, tab.id));
                  }}
                  style={{ backgroundColor: colorVar(tab.color) }}
                  className={swatchClass(health[tab.id]?.reachable ?? true, unknown)}
                />
                <button
                  type="button"
                  aria-current={tab.id === active}
                  title={tab.id}
                  disabled={busy}
                  onClick={() => void show(tab)}
                  className="truncate font-mono disabled:text-fg-subtle"
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
              </span>
            ))}
          </span>
        ))}
        {tabs.length === 0 && <span className="pb-1.5 text-fg-muted">no cluster</span>}
      </nav>
      {painted !== null && (
        <span className="absolute top-full z-30" style={{ left: `${String(paintAt)}px` }}>
          <TabMenu
            tab={painted}
            onDone={() => {
              setPainting('');
            }}
          />
        </span>
      )}
      <span className="flex shrink-0 items-center pb-1">
        <ContextPicker onSwitched={onShown} />
      </span>
      {asking !== null && (
        <dialog
          open
          aria-label={`Close ${asking.context}`}
          className="fixed inset-0 z-40 m-auto w-[26rem] max-w-[calc(100vw-2rem)] rounded border border-warn-line bg-surface p-0 text-fg"
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
    </div>
  );
}

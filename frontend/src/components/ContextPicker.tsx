import { useEffect, useMemo, useRef, useState } from 'react';
import type { ClusterList, ContextList } from '../lib/types';
import { contextGroups, fetchContexts, sameContext } from '../lib/contexts';
import { activateCluster, closeCluster, openCluster, wasCancelled } from '../lib/clusters';
import { forgetTab, tabFor } from '../lib/tabs';
import { reasonOf } from '../lib/object';
import type { Tab } from '../store/clusters';
import { nameOf, useActiveCluster, useTabs } from '../store/clusters';
import { useSettingsStore } from '../store/settings';
import type { OpenContext } from '../lib/settings';
import type { ContextEntry } from '../lib/contexts';
import { askToast, dismissToast, notifyError, notifyOk } from '../store/toasts';
import { useContextList, useContextsStore } from '../store/contexts';
import { sessionExpired } from '../store/session';
import { CONTROL, TAB_CONTROL } from '../lib/controls';
import { useDismissMenu } from '../lib/useDismissMenu';
import KubeconfigDialog from './KubeconfigDialog';
import ClusterSwatch from './ClusterSwatch';
import { useActiveTab } from '../store/clusters';

type PickerLook = 'control' | 'tab';

interface ContextPickerProps {
  onSwitched: () => void;
  look?: PickerLook;
}

function shellClass(look: PickerLook): string {
  if (look === 'tab') {
    return 'flex items-end gap-2';
  }
  return 'flex items-center gap-2';
}

function triggerClass(look: PickerLook): string {
  if (look === 'tab') {
    return `${TAB_CONTROL} cursor-pointer list-none [&::-webkit-details-marker]:hidden`;
  }
  return `${CONTROL} max-w-64 cursor-pointer list-none border-edge-strong bg-surface-raised font-semibold text-fg-strong hover:bg-surface-active [&::-webkit-details-marker]:hidden`;
}

function triggerLabel(look: PickerLook): string {
  if (look === 'tab') {
    return 'Open another cluster';
  }
  return 'Kubernetes context';
}

function currentLabel(list: ContextList, named: string): string {
  if (named !== '') {
    return named;
  }
  if (list.current.name === '') {
    return 'no cluster';
  }
  return list.current.name;
}

const MENU_ROW = 'px-3 py-1.5 text-left whitespace-nowrap hover:bg-surface-active';

const REFRESH_MS = 30000;

const OPEN_BUDGET = '30s';

const RETRY_BASE_MS = 1000;
const RETRY_MAX_MS = 15000;

function replacing(mode: Exclude<OpenContext, 'ask'>, held: string, opened: ClusterList): boolean {
  if (mode !== 'replace') {
    return false;
  }
  return opened.clusters.some((one) => one.active && one.id !== held && held !== '');
}

function retryDelay(attempt: number): number {
  return Math.min(RETRY_MAX_MS, RETRY_BASE_MS * 2 ** attempt);
}

function rowClass(active: boolean): string {
  if (active) {
    return `${MENU_ROW} bg-surface-active text-fg-strong`;
  }
  return `${MENU_ROW} text-fg-soft`;
}

function rowLabel(name: string, already: boolean): string {
  if (already) {
    return `${name} already open`;
  }
  return name;
}

function rowTitle(cluster: string, already: boolean): string {
  if (already) {
    return `${cluster} — already open; this shows that tab`;
  }
  return cluster;
}

function current(active: boolean): 'true' | undefined {
  if (active) {
    return 'true';
  }
  return undefined;
}

export default function ContextPicker({ onSwitched, look = 'control' }: ContextPickerProps) {
  const list = useContextList();
  const tabs = useTabs();
  const openContext = useSettingsStore((state) => state.openContext);
  const rememberChoice = useSettingsStore((state) => state.setOpenContext);
  const [asking, setAsking] = useState<ContextEntry | null>(null);
  const [remember, setRemember] = useState(false);
  const named = useActiveTab()?.label ?? '';
  const shown = useActiveCluster();
  const setList = useContextsStore((state) => state.setList);
  const [busy, setBusy] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);
  const [managing, setManaging] = useState(false);
  const menuRef = useRef<HTMLDetailsElement | null>(null);
  const listRequest = useRef(0);
  const busyRef = useRef(false);

  useDismissMenu(menuRef);

  const groups = useMemo(() => contextGroups(list), [list]);

  useEffect(() => {
    let live = true;
    let timer: ReturnType<typeof setTimeout> | null = null;
    const generation = listRequest.current + 1;
    listRequest.current = generation;
    fetchContexts()
      .then((found) => {
        if (!live || listRequest.current !== generation) {
          return;
        }
        setList(found);
        setLoadError(found.error ?? null);
      })
      .catch((err: unknown) => {
        if (!live || listRequest.current !== generation) {
          return;
        }
        setLoadError(reasonOf(err, 'the context list could not be loaded'));
        timer = setTimeout(() => {
          setAttempt((value) => value + 1);
        }, retryDelay(attempt));
      });
    return () => {
      live = false;
      if (timer !== null) {
        clearTimeout(timer);
      }
    };
  }, [attempt, setList]);

  useEffect(() => {
    let live = true;
    let inFlight = false;
    const timer = setInterval(() => {
      if (busyRef.current || inFlight || sessionExpired()) {
        return;
      }
      inFlight = true;
      const generation = listRequest.current + 1;
      listRequest.current = generation;
      fetchContexts()
        .then((found) => {
          if (live && listRequest.current === generation) {
            setList(found);
          }
        })
        .catch(() => undefined)
        .finally(() => {
          inFlight = false;
        });
    }, REFRESH_MS);
    return () => {
      live = false;
      clearInterval(timer);
    };
  }, [busy, setList]);

  function closeMenu() {
    const menu = menuRef.current;
    if (menu !== null) {
      menu.open = false;
    }
  }

  function handleManage() {
    closeMenu();
    setManaging(true);
  }

  async function focus(tab: Tab) {
    if (tab.id === shown) {
      onSwitched();
      return;
    }
    if (busyRef.current) {
      return;
    }
    busyRef.current = true;
    setBusy(true);
    try {
      await activateCluster(tab.id);
      notifyOk(`Showing ${nameOf(tab)}`);
      onSwitched();
    } catch (err: unknown) {
      notifyError(`Switching to ${nameOf(tab)}: ${reasonOf(err, 'the request failed')}`);
    } finally {
      busyRef.current = false;
      setBusy(false);
    }
  }

  function pick(entry: ContextEntry, newTab: boolean) {
    closeMenu();
    const already = tabFor(tabs, entry.kubeconfig, entry.file, entry.name);
    if (already !== null && !newTab) {
      void focus(already);
      return;
    }
    if (newTab || tabs.length !== 1) {
      void handleChoose(entry, 'new');
      return;
    }
    if (openContext === 'ask') {
      setAsking(entry);
      return;
    }
    void handleChoose(entry, openContext);
  }

  function answer(entry: ContextEntry, mode: Exclude<OpenContext, 'ask'>) {
    setAsking(null);
    if (remember) {
      rememberChoice(mode);
      setRemember(false);
    }
    void handleChoose(entry, mode);
  }

  async function replaced(previous: string) {
    try {
      await closeCluster(previous);
      forgetTab(previous);
    } catch (err: unknown) {
      notifyError(`Closing the tab it replaced: ${reasonOf(err, 'the request failed')}`);
    }
  }

  async function handleChoose(entry: ContextEntry, mode: Exclude<OpenContext, 'ask'>) {
    if (busyRef.current) {
      return;
    }
    busyRef.current = true;
    setBusy(true);
    const generation = listRequest.current + 1;
    listRequest.current = generation;
    const giveUp = new AbortController();
    const waiting = askToast(`Opening ${entry.name}, up to ${OPEN_BUDGET}`, [
      {
        label: 'Cancel',
        run: () => {
          giveUp.abort();
        },
      },
    ]);
    const held = tabs[0]?.id ?? '';
    try {
      const opened = await openCluster(entry.kubeconfig, entry.name, giveUp.signal);
      if (replacing(mode, held, opened)) {
        await replaced(held);
      }
      const found = await fetchContexts();
      if (listRequest.current !== generation) {
        return;
      }
      setList(found);
      notifyOk(`Opened ${entry.name}`);
      onSwitched();
    } catch (err: unknown) {
      if (listRequest.current !== generation) {
        return;
      }
      if (wasCancelled(err)) {
        notifyOk(`Stopped opening ${entry.name}`);
        return;
      }
      notifyError(`Opening ${entry.name}: ${reasonOf(err, 'opening the cluster failed')}`);
    } finally {
      dismissToast(waiting);
      if (listRequest.current === generation) {
        busyRef.current = false;
        setBusy(false);
      }
    }
  }

  function handleChanged(found: ContextList) {
    listRequest.current += 1;
    setList(found);
    setLoadError(found.error ?? null);
  }

  function retryLoad() {
    setAttempt((value) => value + 1);
  }

  function manageButton() {
    return (
      <button
        type="button"
        title="The kubeconfigs spinoza reads"
        onClick={() => {
          setManaging(true);
        }}
        className={`${CONTROL} border-edge-strong text-fg-soft hover:bg-surface-active`}
      >
        Kubeconfigs
      </button>
    );
  }

  function manageEntry() {
    return (
      <button type="button" onClick={handleManage} className={`${MENU_ROW} border-t border-edge`}>
        Manage kubeconfigs
      </button>
    );
  }

  function choice() {
    if (asking === null) {
      return null;
    }
    const entry = asking;
    return (
      <dialog
        open
        aria-label={`Open ${entry.name}`}
        className="fixed inset-0 z-40 m-auto w-[28rem] max-w-[calc(100vw-2rem)] rounded border border-edge-strong bg-surface p-0 text-fg"
      >
        <div className="border-b border-edge px-3 py-2 text-xs font-semibold tracking-wide text-fg-strong uppercase">
          Open {entry.name}
        </div>
        <div className="p-3 text-xs">
          <p className="text-fg-soft">One cluster is open.</p>
          <label className="mt-3 flex items-center gap-2 text-fg-soft">
            <input
              type="checkbox"
              checked={remember}
              onChange={(event) => {
                setRemember(event.target.checked);
              }}
            />
            Do not ask again
          </label>
          <div className="mt-3 flex items-center justify-end gap-2">
            <button
              type="button"
              onClick={() => {
                setAsking(null);
                setRemember(false);
              }}
              className="rounded border border-edge-strong px-2 py-1 text-fg-soft hover:bg-surface-active"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={() => {
                answer(entry, 'replace');
              }}
              className="rounded border border-edge-strong px-2 py-1 text-fg hover:bg-surface-active"
            >
              Replace this tab
            </button>
            <button
              type="button"
              onClick={() => {
                answer(entry, 'new');
              }}
              className="rounded border border-edge-strong px-2 py-1 text-fg hover:bg-surface-active"
            >
              Open a new tab
            </button>
          </div>
        </div>
      </dialog>
    );
  }

  function dialog() {
    return (
      <KubeconfigDialog
        open={managing}
        kubeconfigs={list.kubeconfigs}
        onChanged={handleChanged}
        onClose={() => {
          setManaging(false);
        }}
      />
    );
  }

  if (groups.length === 0 && loadError !== null) {
    return (
      <span className={shellClass(look)}>
        <span role="status" className="max-w-md truncate text-error">
          no cluster context: {loadError}
        </span>
        <button
          type="button"
          onClick={retryLoad}
          className={`${CONTROL} border-edge-strong text-fg hover:bg-surface-active`}
        >
          Retry
        </button>
        {manageButton()}
        {dialog()}
      </span>
    );
  }

  function triggerBody() {
    if (look === 'tab') {
      return <span aria-hidden="true">+</span>;
    }
    return (
      <>
        <ClusterSwatch />
        <span className="truncate">{currentLabel(list, named)}</span>
        <span aria-hidden="true" className="ml-auto pl-2 text-fg-muted">
          ▾
        </span>
      </>
    );
  }

  function triggerTitle() {
    if (look === 'tab') {
      return 'Open another cluster';
    }
    return currentLabel(list, named);
  }

  if (groups.length === 0) {
    return (
      <span className={shellClass(look)}>
        {look !== 'tab' && (
          <span className="font-semibold text-fg-strong">{currentLabel(list, named)}</span>
        )}
        {manageButton()}
        {dialog()}
      </span>
    );
  }

  return (
    <span className={shellClass(look)}>
      <details ref={menuRef} className="relative">
        <summary
          aria-label={triggerLabel(look)}
          title={triggerTitle()}
          className={triggerClass(look)}
        >
          {triggerBody()}
        </summary>
        <div
          role="group"
          aria-label="Contexts you can open"
          className="absolute left-0 z-30 mt-1 flex max-h-[70vh] w-max max-w-[36rem] min-w-full flex-col overflow-y-auto rounded border border-edge-strong bg-surface-raised shadow"
        >
          <p className="border-b border-edge px-3 py-1 text-[11px] text-fg-muted">
            Contexts you can open. One already open shows its tab instead.
          </p>
          {groups.map((group) => (
            <div key={group.path} className="flex flex-col">
              <div
                title={group.path}
                className="truncate px-3 py-1 text-[11px] font-semibold tracking-wide text-fg-muted uppercase"
              >
                {group.label}
              </div>
              {group.error !== undefined && (
                <div className="px-3 py-1 text-warn-muted">{group.error}</div>
              )}
              {group.entries.map((entry) => {
                const already = tabFor(tabs, entry.kubeconfig, entry.file, entry.name);
                return (
                  <button
                    key={entry.value}
                    type="button"
                    aria-current={current(sameContext(entry, list.current))}
                    aria-label={rowLabel(entry.name, already !== null)}
                    title={rowTitle(entry.cluster, already !== null)}
                    onClick={(event) => {
                      pick(entry, event.metaKey || event.ctrlKey);
                    }}
                    className={rowClass(sameContext(entry, list.current))}
                  >
                    {entry.name}
                    {already !== null && (
                      <span className="ml-2 text-[11px] text-fg-muted">already open</span>
                    )}
                  </button>
                );
              })}
            </div>
          ))}
          {manageEntry()}
        </div>
      </details>
      {busy && <span className="text-fg-muted">opening</span>}
      {choice()}
      {dialog()}
    </span>
  );
}

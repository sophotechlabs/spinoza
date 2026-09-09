import { useEffect, useMemo, useRef, useState } from 'react';
import type { ClusterList, ContextList } from '../lib/types';
import { contextGroups, fetchContexts, sameContext } from '../lib/contexts';
import { closeCluster, openCluster, wasCancelled } from '../lib/clusters';
import { forgetTab } from '../lib/tabs';
import { useTabs } from '../store/clusters';
import { useSettingsStore } from '../store/settings';
import type { OpenContext } from '../lib/settings';
import type { ContextEntry } from '../lib/contexts';
import { askToast, dismissToast, notifyError, notifyOk } from '../store/toasts';
import { useContextList, useContextsStore } from '../store/contexts';
import { sessionExpired } from '../store/session';
import { CONTROL } from '../lib/controls';
import { useDismissMenu } from '../lib/useDismissMenu';
import KubeconfigDialog from './KubeconfigDialog';
import ClusterSwatch from './ClusterSwatch';
import { useActiveTab } from '../store/clusters';

interface ContextPickerProps {
  onSwitched: () => void;
}

const MENU_ROW = 'px-3 py-1.5 text-left whitespace-nowrap hover:bg-surface-active';

const REFRESH_MS = 30000;

const OPEN_BUDGET = '30s';

const RETRY_BASE_MS = 1000;
const RETRY_MAX_MS = 15000;

function errorMessage(err: unknown, fallback: string): string {
  if (err instanceof Error) {
    return err.message;
  }
  return fallback;
}

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

function current(active: boolean): 'true' | undefined {
  if (active) {
    return 'true';
  }
  return undefined;
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

export default function ContextPicker({ onSwitched }: ContextPickerProps) {
  const list = useContextList();
  const tabs = useTabs();
  const openContext = useSettingsStore((state) => state.openContext);
  const rememberChoice = useSettingsStore((state) => state.setOpenContext);
  const [asking, setAsking] = useState<ContextEntry | null>(null);
  const [remember, setRemember] = useState(false);
  const named = useActiveTab()?.label ?? '';
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
        setLoadError(errorMessage(err, 'the context list could not be loaded'));
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

  function pick(entry: ContextEntry, newTab: boolean) {
    closeMenu();
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
      notifyError(`Closing the tab it replaced: ${errorMessage(err, 'the request failed')}`);
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
      notifyError(`Opening ${entry.name}: ${errorMessage(err, 'opening the cluster failed')}`);
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
        className="fixed inset-0 z-40 m-auto w-[28rem] rounded border border-edge-strong bg-surface p-0 text-fg"
      >
        <div className="border-b border-edge px-3 py-2 text-xs font-semibold tracking-wide text-fg-strong uppercase">
          Open {entry.name}
        </div>
        <div className="p-3 text-xs">
          <p className="text-fg-soft">
            One cluster is open. Replace it, or keep it and open this one beside it?
          </p>
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
      <span className="flex items-center gap-2">
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

  if (groups.length === 0) {
    return (
      <span className="flex items-center gap-2">
        <span className="font-semibold text-fg-strong">{currentLabel(list, named)}</span>
        {manageButton()}
        {dialog()}
      </span>
    );
  }

  return (
    <span className="flex items-center gap-2">
      <details ref={menuRef} className="relative">
        <summary
          aria-label="Kubernetes context"
          title={currentLabel(list, named)}
          className={`${CONTROL} max-w-64 cursor-pointer list-none border-edge-strong bg-surface-raised font-semibold text-fg-strong hover:bg-surface-active [&::-webkit-details-marker]:hidden`}
        >
          <ClusterSwatch />
          <span className="truncate">{currentLabel(list, named)}</span>
          <span aria-hidden="true" className="ml-auto pl-2 text-fg-muted">
            ▾
          </span>
        </summary>
        <div className="absolute left-0 z-30 mt-1 flex max-h-[70vh] w-max max-w-[36rem] min-w-full flex-col overflow-y-auto rounded border border-edge-strong bg-surface-raised shadow">
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
              {group.entries.map((entry) => (
                <button
                  key={entry.value}
                  type="button"
                  aria-current={current(sameContext(entry, list.current))}
                  title={entry.cluster}
                  onClick={(event) => {
                    pick(entry, event.metaKey || event.ctrlKey);
                  }}
                  className={rowClass(sameContext(entry, list.current))}
                >
                  {entry.name}
                </button>
              ))}
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

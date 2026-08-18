import { useEffect, useMemo, useRef, useState } from 'react';
import type { ContextList } from '../lib/types';
import { contextGroups, fetchContexts, sameContext, switchContext } from '../lib/contexts';
import type { ContextEntry } from '../lib/contexts';
import { notifyError, notifyOk } from '../store/toasts';
import { useContextList, useContextsStore } from '../store/contexts';
import { sessionExpired } from '../store/session';
import { CONTROL } from '../lib/controls';
import { useDismissMenu } from '../lib/useDismissMenu';
import KubeconfigDialog from './KubeconfigDialog';

interface ContextPickerProps {
  onSwitched: () => void;
}

const MENU_ROW = 'px-3 py-1.5 text-left whitespace-nowrap hover:bg-surface-active';

const REFRESH_MS = 30000;

const RETRY_BASE_MS = 1000;
const RETRY_MAX_MS = 15000;

function errorMessage(err: unknown, fallback: string): string {
  if (err instanceof Error) {
    return err.message;
  }
  return fallback;
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

function currentLabel(list: ContextList): string {
  if (list.current.name === '') {
    return 'no cluster';
  }
  return list.current.name;
}

export default function ContextPicker({ onSwitched }: ContextPickerProps) {
  const list = useContextList();
  const setList = useContextsStore((state) => state.setList);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);
  const [managing, setManaging] = useState(false);
  const menuRef = useRef<HTMLDetailsElement | null>(null);

  useDismissMenu(menuRef);

  const groups = useMemo(() => contextGroups(list), [list]);

  useEffect(() => {
    let live = true;
    let timer: ReturnType<typeof setTimeout> | null = null;
    fetchContexts()
      .then((found) => {
        if (!live) {
          return;
        }
        setList(found);
        setLoadError(found.error ?? null);
      })
      .catch((err: unknown) => {
        if (!live) {
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
    const timer = setInterval(() => {
      if (busy || sessionExpired()) {
        return;
      }
      fetchContexts()
        .then((found) => {
          setList(found);
        })
        .catch(() => undefined);
    }, REFRESH_MS);
    return () => {
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

  async function handleChoose(entry: ContextEntry) {
    closeMenu();
    if (busy || sameContext(entry, list.current)) {
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const found = await switchContext(entry);
      setList(found);
      notifyOk(`Switched to ${found.current.name}`);
      onSwitched();
    } catch (err: unknown) {
      const message = errorMessage(err, 'switching context failed');
      setError(message);
      notifyError(`Switching to ${entry.name}: ${message}`);
    } finally {
      setBusy(false);
    }
  }

  function handleChanged(found: ContextList) {
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
        <span className="font-semibold text-fg-strong">{currentLabel(list)}</span>
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
          title={currentLabel(list)}
          className={`${CONTROL} max-w-64 cursor-pointer list-none border-edge-strong bg-surface-raised font-semibold text-fg-strong hover:bg-surface-active [&::-webkit-details-marker]:hidden`}
        >
          <span className="truncate">{currentLabel(list)}</span>
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
                  onClick={() => void handleChoose(entry)}
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
      {busy && <span className="text-fg-muted">switching</span>}
      {error !== null && (
        <span role="status" className="max-w-md truncate text-error">
          {error}
        </span>
      )}
      {dialog()}
    </span>
  );
}

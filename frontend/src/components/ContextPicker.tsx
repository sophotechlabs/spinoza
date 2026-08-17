import { useEffect, useMemo, useState } from 'react';
import type { ContextList } from '../lib/types';
import {
  contextGroups,
  entryFor,
  everyContext,
  fetchContexts,
  sameContext,
  switchContext,
} from '../lib/contexts';
import { notifyError, notifyOk } from '../store/toasts';
import { useContextList, useContextsStore } from '../store/contexts';
import { sessionExpired } from '../store/session';
import { CONTROL } from '../lib/controls';
import KubeconfigDialog from './KubeconfigDialog';

interface ContextPickerProps {
  onSwitched: () => void;
}

const REFRESH_MS = 30000;

const RETRY_BASE_MS = 1000;
const RETRY_MAX_MS = 15000;

const UNLISTED = 'unlisted';

function errorMessage(err: unknown, fallback: string): string {
  if (err instanceof Error) {
    return err.message;
  }
  return fallback;
}

function retryDelay(attempt: number): number {
  return Math.min(RETRY_MAX_MS, RETRY_BASE_MS * 2 ** attempt);
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

  const groups = useMemo(() => contextGroups(list), [list]);
  const selected = everyContext(groups).find((entry) => sameContext(entry, list.current));

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

  async function handleChange(event: React.ChangeEvent<HTMLSelectElement>) {
    const entry = entryFor(groups, event.target.value);
    if (entry === undefined || sameContext(entry, list.current)) {
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
      <select
        aria-label="Kubernetes context"
        value={selected?.value ?? UNLISTED}
        onChange={(event) => void handleChange(event)}
        disabled={busy}
        className={`${CONTROL} max-w-64 border-edge-strong bg-surface-raised font-semibold text-fg-strong disabled:text-fg-subtle`}
      >
        {selected === undefined && (
          <option value={UNLISTED} disabled>
            {currentLabel(list)}
          </option>
        )}
        {groups.map((group) => (
          <optgroup key={group.path} label={group.label}>
            {group.error !== undefined && (
              <option value={`${group.path}-error`} disabled>
                {group.error}
              </option>
            )}
            {group.entries.map((entry) => (
              <option key={entry.value} value={entry.value} title={entry.cluster}>
                {entry.name}
              </option>
            ))}
          </optgroup>
        ))}
      </select>
      {manageButton()}
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

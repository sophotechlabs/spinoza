import { useEffect, useState } from 'react';
import { fetchContexts, switchContext } from '../lib/contexts';
import { notifyError, notifyOk } from '../store/toasts';

interface ContextPickerProps {
  onSwitched: () => void;
}

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

export default function ContextPicker({ onSwitched }: ContextPickerProps) {
  const [contexts, setContexts] = useState<string[]>([]);
  const [current, setCurrent] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let live = true;
    let timer: ReturnType<typeof setTimeout> | null = null;
    fetchContexts()
      .then((list) => {
        if (!live) {
          return;
        }
        setContexts(list.contexts);
        setCurrent(list.current);
        setLoadError(list.error ?? null);
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
  }, [attempt]);

  async function handleChange(event: React.ChangeEvent<HTMLSelectElement>) {
    const name = event.target.value;
    if (name === current) {
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const list = await switchContext(name);
      setCurrent(list.current);
      notifyOk(`Switched to ${list.current}`);
      onSwitched();
    } catch (err: unknown) {
      const message = errorMessage(err, 'switching context failed');
      setError(message);
      notifyError(`Switching to ${name}: ${message}`);
    } finally {
      setBusy(false);
    }
  }

  function retryLoad() {
    setAttempt((value) => value + 1);
  }

  if (contexts.length === 0 && loadError !== null) {
    return (
      <span className="flex items-center gap-2">
        <span role="status" className="max-w-md truncate text-error">
          no cluster context — {loadError}
        </span>
        <button
          type="button"
          onClick={retryLoad}
          className="rounded border border-edge-strong px-1.5 py-0.5 text-fg hover:bg-surface-active"
        >
          Retry
        </button>
      </span>
    );
  }

  if (contexts.length === 0) {
    return <span className="font-semibold text-fg-strong">{current}</span>;
  }

  return (
    <span className="flex items-center gap-2">
      <select
        aria-label="Kubernetes context"
        value={current}
        onChange={(event) => void handleChange(event)}
        disabled={busy}
        className="rounded border border-edge-strong bg-surface-raised px-1.5 py-0.5 font-semibold text-fg-strong disabled:text-fg-subtle"
      >
        {contexts.map((name) => (
          <option key={name} value={name}>
            {name}
          </option>
        ))}
      </select>
      {busy && <span className="text-fg-muted">switching…</span>}
      {error !== null && (
        <span role="status" className="max-w-md truncate text-error">
          {error}
        </span>
      )}
    </span>
  );
}

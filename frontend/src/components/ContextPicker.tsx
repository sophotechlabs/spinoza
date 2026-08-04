import { useEffect, useState } from 'react';
import { fetchContexts, switchContext } from '../lib/contexts';

interface ContextPickerProps {
  onSwitched: () => void;
}

export default function ContextPicker({ onSwitched }: ContextPickerProps) {
  const [contexts, setContexts] = useState<string[]>([]);
  const [current, setCurrent] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    fetchContexts()
      .then((list) => {
        if (!live) {
          return;
        }
        setContexts(list.contexts);
        setCurrent(list.current);
      })
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, []);

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
      onSwitched();
    } catch (err: unknown) {
      if (err instanceof Error) {
        setError(err.message);
      } else {
        setError('switching context failed');
      }
    } finally {
      setBusy(false);
    }
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
      {error !== null && <span className="max-w-md truncate text-error">{error}</span>}
    </span>
  );
}

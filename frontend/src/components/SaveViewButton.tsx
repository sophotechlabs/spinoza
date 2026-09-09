import { useEffect, useState } from 'react';
import type { SavedView } from '../lib/types';
import { fetchSavedViews, saveView } from '../lib/savedViews';
import { notifyError, notifyOk } from '../store/toasts';
import { CONTROL } from '../lib/controls';

interface SaveViewButtonProps {
  view: string;
  resource?: string;
  namespace?: string;
  filter: string;
  columns: string[];
}

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'that view was not saved';
}

export default function SaveViewButton({
  view,
  resource,
  namespace,
  filter,
  columns,
}: SaveViewButtonProps) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState('');
  const [shared, setShared] = useState(false);
  const [busy, setBusy] = useState(false);
  const [mayShare, setMayShare] = useState(false);

  useEffect(() => {
    if (!open) {
      return;
    }
    let live = true;
    fetchSavedViews()
      .then((page) => {
        if (live) {
          setMayShare(page.mayShare === true);
        }
      })
      .catch(() => {
        if (live) {
          setMayShare(false);
        }
      });
    return () => {
      live = false;
    };
  }, [open]);

  async function keep() {
    setBusy(true);
    const wanted: SavedView = {
      id: '',
      name: name.trim(),
      view,
      resource,
      namespace,
      filter,
      columns,
      shared,
    };
    try {
      const kept = await saveView(wanted);
      notifyOk(`saved "${kept.name}"; the command palette lists it`);
      setOpen(false);
      setName('');
      setShared(false);
    } catch (err: unknown) {
      notifyError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  if (!open) {
    return (
      <button
        type="button"
        onClick={() => {
          setOpen(true);
        }}
        className={`${CONTROL} border-edge text-fg-soft hover:bg-surface-raised`}
      >
        Save this view
      </button>
    );
  }

  return (
    <span className="flex items-center gap-1">
      <input
        aria-label="Name for this view"
        placeholder="name it"
        value={name}
        onChange={(event) => {
          setName(event.target.value);
        }}
        onKeyDown={(event) => {
          if (event.key === 'Enter' && name.trim() !== '') {
            void keep();
          }
          if (event.key === 'Escape') {
            setOpen(false);
          }
        }}
        className="w-36 rounded border border-edge-strong bg-surface px-2 py-1 text-fg"
      />
      {mayShare && (
        <label className="flex items-center gap-1 text-fg-muted">
          <input
            type="checkbox"
            checked={shared}
            onChange={(event) => {
              setShared(event.target.checked);
            }}
          />
          everybody
        </label>
      )}
      <button
        type="button"
        disabled={busy || name.trim() === ''}
        onClick={() => {
          void keep();
        }}
        className={`${CONTROL} border-edge-strong text-fg hover:bg-surface-active disabled:opacity-50`}
      >
        Save
      </button>
      <button
        type="button"
        onClick={() => {
          setOpen(false);
        }}
        className={`${CONTROL} border-edge text-fg-soft hover:bg-surface-raised`}
      >
        Cancel
      </button>
    </span>
  );
}

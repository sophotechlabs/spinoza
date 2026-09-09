import { useCallback, useState } from 'react';
import type { SavedView } from '../lib/types';
import { describeView, fetchSavedViews, forgetView } from '../lib/savedViews';
import { usePoll } from '../lib/usePoll';
import { notifyError } from '../store/toasts';
import { CONTROL } from '../lib/controls';

const VIEWS_POLL_MS = 60000;

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'that view was not forgotten';
}

function ownership(view: SavedView): string {
  if (view.shared === true) {
    return 'everybody';
  }
  return 'yours';
}

interface SavedViewsListProps {
  active: boolean;
}

export default function SavedViewsList({ active }: SavedViewsListProps) {
  const load = useCallback(() => fetchSavedViews(), []);
  const {
    data: page,
    error,
    reload,
  } = usePoll(load, { intervalMs: VIEWS_POLL_MS, enabled: active });
  const [busy, setBusy] = useState<string | null>(null);

  if (page === null) {
    if (error !== null) {
      return <span className="text-error">{error}</span>;
    }
    return <span className="text-fg-muted">reading them…</span>;
  }

  if (page.views.length === 0) {
    return <span className="text-fg-muted">None yet. Save this view from any resource table.</span>;
  }

  return (
    <ul className="w-full">
      {page.views.map((one) => (
        <li key={one.id} className="flex items-baseline gap-2 py-0.5">
          <span className="text-fg">{one.name}</span>
          <span className="truncate text-fg-muted">{describeView(one)}</span>
          <span className="text-fg-subtle">{ownership(one)}</span>
          <button
            type="button"
            disabled={busy === one.id || (one.shared === true && page.mayShare !== true)}
            onClick={() => {
              setBusy(one.id);
              forgetView(one.id, one.shared === true)
                .then(() => {
                  reload();
                })
                .catch((err: unknown) => {
                  notifyError(errorMessage(err));
                })
                .finally(() => {
                  setBusy(null);
                });
            }}
            className={`${CONTROL} ml-auto border-edge text-fg-soft hover:bg-surface-raised disabled:opacity-50`}
          >
            Forget
          </button>
        </li>
      ))}
    </ul>
  );
}

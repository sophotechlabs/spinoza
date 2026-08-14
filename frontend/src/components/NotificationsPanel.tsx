import { useState } from 'react';
import type { ObjectRef } from '../lib/types';
import type { Notification, ToastTone } from '../store/toasts';
import { useToastsStore } from '../store/toasts';
import { clock } from '../lib/time';

type Jump = (ref: ObjectRef) => void;

interface NotificationsPanelProps {
  onSelectObject: Jump;
}

type Filter = 'all' | ToastTone;

const FILTERS: { id: Filter; label: string }[] = [
  { id: 'all', label: 'All' },
  { id: 'ok', label: 'Done' },
  { id: 'warn', label: 'Warnings' },
  { id: 'error', label: 'Failures' },
];

function toneClass(tone: ToastTone): string {
  if (tone === 'error') {
    return 'text-error';
  }
  if (tone === 'warn') {
    return 'text-warn';
  }
  return 'text-ok';
}

function filterClass(on: boolean): string {
  if (on) {
    return 'rounded border border-edge-strong bg-surface-active px-1.5 py-0.5 text-fg-strong';
  }
  return 'rounded border border-edge px-1.5 py-0.5 text-fg-muted hover:bg-surface-raised';
}

function matching(history: Notification[], filter: Filter): Notification[] {
  const newest = [...history].reverse();
  if (filter === 'all') {
    return newest;
  }
  return newest.filter((note) => note.tone === filter);
}

function whereFrom(ref: ObjectRef): string {
  if (ref.namespace === '') {
    return `${ref.resource}/${ref.name}`;
  }
  return `${ref.resource}/${ref.namespace}/${ref.name}`;
}

function Row({ note, onSelectObject }: { note: Notification; onSelectObject: Jump }) {
  const ref = note.ref;
  return (
    <li className="flex items-baseline gap-2 border-b border-edge px-2 py-1">
      <span className="shrink-0 font-mono text-fg-muted">{clock(note.at)}</span>
      <span className={`shrink-0 ${toneClass(note.tone)}`} role="img" aria-label={note.tone}>
        ●
      </span>
      <span className="break-words text-fg-soft">{note.message}</span>
      {ref !== undefined && (
        <button
          type="button"
          onClick={() => {
            onSelectObject(ref);
          }}
          className="ml-auto shrink-0 truncate text-fg-muted hover:text-fg-strong hover:underline"
        >
          {whereFrom(ref)}
        </button>
      )}
    </li>
  );
}

export default function NotificationsPanel({ onSelectObject }: NotificationsPanelProps) {
  const history = useToastsStore((state) => state.history);
  const forget = useToastsStore((state) => state.clearHistory);
  const [filter, setFilter] = useState<Filter>('all');

  const shown = matching(history, filter);

  return (
    <div className="flex min-h-0 flex-col text-xs">
      <div className="flex shrink-0 items-center gap-1.5 border-b border-edge px-2 py-1.5">
        {FILTERS.map((one) => (
          <button
            key={one.id}
            type="button"
            aria-pressed={filter === one.id}
            onClick={() => {
              setFilter(one.id);
            }}
            className={filterClass(filter === one.id)}
          >
            {one.label}
          </button>
        ))}
        <button
          type="button"
          disabled={history.length === 0}
          onClick={forget}
          className="ml-auto rounded border border-edge-strong px-1.5 py-0.5 text-fg-soft hover:bg-surface-active disabled:text-fg-faint"
        >
          Clear
        </button>
      </div>
      {history.length > 0 && (
        <p className="shrink-0 border-b border-edge px-2 py-1 text-fg-muted">
          {history.length} since this cluster was opened, newest first
        </p>
      )}
      {shown.length === 0 && <p className="p-3 text-fg-muted">Nothing to show here yet.</p>}
      <ul className="min-h-0 flex-1 overflow-y-auto">
        {shown.map((note) => (
          <Row key={note.id} note={note} onSelectObject={onSelectObject} />
        ))}
      </ul>
    </div>
  );
}

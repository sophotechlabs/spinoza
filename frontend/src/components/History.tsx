import { useState } from 'react';
import type { HistoryEntry, ObjectRef } from '../lib/types';
import {
  HISTORY_LIMIT,
  SOURCES,
  clearFailure,
  detailText,
  forgetHistory,
  outcomeClass,
  outcomeLabel,
  recordFailure,
  refOf,
  scopeLabel,
  sourceLabel,
  targetLabel,
  useHistory,
  verbLabel,
  when,
} from '../lib/history';
import type { HistorySource } from '../lib/history';
import { CONTROL } from '../lib/controls';
import { recordCluster } from '../lib/clusters';
import { useActiveTab } from '../store/clusters';
import { notifyError, notifyOk } from '../store/toasts';
import { useNow } from '../lib/useNow';
import LoadWarning from './LoadWarning';
import StaleBanner from './StaleBanner';
import Loading from './Loading';

interface HistoryProps {
  onOpen: (ref: ObjectRef) => void;
}

const HEADERS = [
  { id: 'at', label: 'When', width: 'w-32' },
  { id: 'verb', label: 'Did', width: 'w-28' },
  { id: 'target', label: 'To', width: 'w-72' },
  { id: 'namespace', label: 'Namespace', width: 'w-40' },
  { id: 'outcome', label: 'Outcome', width: 'w-24' },
  { id: 'message', label: 'Detail', width: 'w-96' },
];

function Target({ entry, onOpen }: { entry: HistoryEntry; onOpen: (ref: ObjectRef) => void }) {
  const ref = refOf(entry);
  if (ref === null) {
    return <span className="truncate text-fg-strong">{targetLabel(entry)}</span>;
  }
  return (
    <button
      type="button"
      onClick={() => {
        onOpen(ref);
      }}
      className="max-w-full truncate text-fg-strong hover:underline"
    >
      {targetLabel(entry)}
    </button>
  );
}

function Row({
  entry,
  now,
  onOpen,
}: {
  entry: HistoryEntry;
  now: number;
  onOpen: (ref: ObjectRef) => void;
}) {
  return (
    <tr className="border-t border-edge hover:bg-surface-raised">
      <td className="truncate px-2 py-1 text-fg-muted" title={entry.at}>
        {when(entry.at, now)}
      </td>
      <td className="truncate px-2 py-1 text-fg-soft">{verbLabel(entry)}</td>
      <td className="truncate px-2 py-1">
        <Target entry={entry} onOpen={onOpen} />
      </td>
      <td className="truncate px-2 py-1 text-fg-muted">{scopeLabel(entry)}</td>
      <td className={`truncate px-2 py-1 ${outcomeClass(entry.outcome)}`}>
        {entry.source === 'change' ? '' : outcomeLabel(entry.outcome)}
      </td>
      <td className="truncate px-2 py-1 text-fg-muted" title={detailText(entry)}>
        {detailText(entry)}
      </td>
    </tr>
  );
}

const KIND_SETS = [
  { id: '', label: 'Recording nothing' },
  { id: 'workloads', label: 'Recording workloads' },
  { id: 'wide', label: 'Recording workloads, network and GitOps' },
];

function Recording() {
  const tab = useActiveTab();
  const [busy, setBusy] = useState(false);
  if (tab === null) {
    return null;
  }
  const on = tab.id;

  async function change(kinds: string) {
    setBusy(true);
    try {
      await recordCluster(on, kinds);
    } catch (err: unknown) {
      notifyError(recordFailure(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <label className="flex items-center gap-1.5 text-fg-soft">
      <span className="sr-only">What to record</span>
      <select
        aria-label="What to record"
        value={tab.timeline}
        disabled={busy}
        onChange={(event) => {
          void change(event.target.value);
        }}
        className={`${CONTROL} border-edge-strong bg-surface text-fg-soft`}
      >
        {KIND_SETS.map((one) => (
          <option key={one.id} value={one.id}>
            {one.label}
          </option>
        ))}
      </select>
    </label>
  );
}

function nothingYet(source: HistorySource): string {
  if (source === 'change') {
    return 'Nothing has changed on this cluster since spinoza started watching it.';
  }
  if (source === 'action') {
    return 'Spinoza has not changed anything on this cluster yet.';
  }
  return 'There is nothing here yet.';
}

export default function History({ onOpen }: HistoryProps) {
  const [source, setSource] = useState<HistorySource>('all');
  const { data, error, reload } = useHistory(source);
  const [clearing, setClearing] = useState(false);
  const now = useNow();

  if (data === null) {
    if (error !== null) {
      return (
        <div className="flex h-full items-center justify-center text-xs text-error">{error}</div>
      );
    }
    return <Loading what="history" />;
  }

  async function handleClear() {
    setClearing(true);
    try {
      await forgetHistory();
      notifyOk('History cleared');
      reload();
    } catch (err: unknown) {
      notifyError(clearFailure(err));
    } finally {
      setClearing(false);
    }
  }

  const notRecording = data.reason ?? '';
  return (
    <div className="flex h-full min-h-0 flex-col text-xs">
      {error !== null && <StaleBanner what="History" message={error} onRetry={reload} />}
      {notRecording !== '' && <LoadWarning message={notRecording} />}
      <div className="flex shrink-0 items-center gap-2 border-b border-edge px-2 py-1.5">
        <label className="flex items-center gap-1.5 text-fg-soft">
          Showing
          <select
            aria-label="What to show"
            value={source}
            onChange={(event) => {
              setSource(event.target.value as HistorySource);
            }}
            className={`${CONTROL} border-edge-strong bg-surface text-fg-soft`}
          >
            {SOURCES.map((one) => (
              <option key={one} value={one}>
                {sourceLabel(one)}
              </option>
            ))}
          </select>
        </label>
        <Recording />
        {data.more && <span className="text-fg-muted">showing the newest {HISTORY_LIMIT}</span>}
        {(data.dropped ?? 0) > 0 && (
          <span className="text-warn">
            {data.dropped} changes came in faster than they could be written and were not kept
          </span>
        )}
        <button
          type="button"
          disabled={clearing || data.entries.length === 0}
          onClick={() => {
            void handleClear();
          }}
          className={`${CONTROL} ml-auto border-edge-strong text-fg-soft hover:bg-surface-active disabled:opacity-50`}
        >
          Clear
        </button>
      </div>
      {data.entries.length === 0 && (
        <div className="flex flex-1 items-center justify-center text-fg-muted">
          {nothingYet(source)}
        </div>
      )}
      {data.entries.length > 0 && (
        <div className="min-h-0 flex-1 overflow-auto">
          <table className="w-full table-fixed border-collapse text-left whitespace-nowrap">
            <thead className="sticky top-0 z-10 bg-surface-raised text-fg-muted">
              <tr className="border-b border-edge">
                {HEADERS.map((header) => (
                  <th key={header.id} className={`px-2 py-1 font-medium ${header.width}`}>
                    {header.label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {data.entries.map((entry) => (
                <Row key={entry.id} entry={entry} now={now} onOpen={onOpen} />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

import { useState } from 'react';
import type { HistoryEntry, ObjectRef } from '../lib/types';
import {
  HISTORY_LIMIT,
  clearFailure,
  detailText,
  forgetHistory,
  outcomeClass,
  outcomeLabel,
  refOf,
  scopeLabel,
  targetLabel,
  useHistory,
  when,
} from '../lib/history';
import { CONTROL } from '../lib/controls';
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
      <td className="truncate px-2 py-1 text-fg-soft">{entry.verb}</td>
      <td className="truncate px-2 py-1">
        <Target entry={entry} onOpen={onOpen} />
      </td>
      <td className="truncate px-2 py-1 text-fg-muted">{scopeLabel(entry)}</td>
      <td className={`truncate px-2 py-1 ${outcomeClass(entry.outcome)}`}>
        {outcomeLabel(entry.outcome)}
      </td>
      <td className="truncate px-2 py-1 text-fg-muted" title={detailText(entry)}>
        {detailText(entry)}
      </td>
    </tr>
  );
}

export default function History({ onOpen }: HistoryProps) {
  const { data, error, reload } = useHistory();
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
        <span className="text-fg-soft">What spinoza did on this cluster</span>
        {data.more && <span className="text-fg-muted">showing the newest {HISTORY_LIMIT}</span>}
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
          Spinoza has not changed anything on this cluster yet.
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

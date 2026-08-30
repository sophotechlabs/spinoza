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
  cursorOf,
  fetchHistory,
  foldRepeats,
  olderFailure,
  reachable,
  repeatLabel,
  sourceLabel,
  targetLabel,
  wasText,
  useHistory,
  useMemory,
  verbLabel,
  when,
} from '../lib/history';
import type { FoldedEntry, HistorySource } from '../lib/history';
import { CONTROL } from '../lib/controls';
import { recordCluster } from '../lib/clusters';
import { nameOf, tabOn, useActiveTab, useClustersStore, useTabStrip } from '../store/clusters';
import { colorVar } from '../lib/clusterColor';
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
  { id: 'cluster', label: 'Cluster', width: 'w-40', fleet: true },
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
  folded,
  now,
  fleet,
  onOpen,
}: {
  folded: FoldedEntry;
  now: number;
  fleet: boolean;
  onOpen: (ref: ObjectRef) => void;
}) {
  const entry = folded.entry;
  const repeats = repeatLabel(folded);
  return (
    <tr className="border-t border-edge hover:bg-surface-raised">
      <td className="truncate px-2 py-1 text-fg-muted" title={entry.at}>
        {when(entry.at, now)}
      </td>
      {fleet && (
        <td className="truncate px-2 py-1">
          <OnCluster cluster={entry.cluster ?? ''} />
        </td>
      )}
      <td className="truncate px-2 py-1 text-fg-soft">{verbLabel(entry)}</td>
      <td className="truncate px-2 py-1">
        <Target entry={entry} onOpen={onOpen} />
      </td>
      <td className="truncate px-2 py-1 text-fg-muted">{scopeLabel(entry)}</td>
      <td className={`truncate px-2 py-1 ${outcomeClass(entry.outcome)}`}>
        {entry.source === 'change' ? '' : outcomeLabel(entry.outcome)}
      </td>
      <td className="truncate px-2 py-1 text-fg-muted" title={detailText(entry)}>
        {wasText(entry)}
        {detailText(entry)}
        {repeats !== '' && <span className="ml-2 text-fg-faint">· {repeats}</span>}
      </td>
    </tr>
  );
}

const KIND_SETS = [
  { id: '', label: 'Recording nothing' },
  { id: 'workloads', label: 'Recording workloads' },
  { id: 'wide', label: 'Recording workloads, network and GitOps' },
];

function OnCluster({ cluster }: { cluster: string }) {
  const tab = useClustersStore((state) => tabOn(state.tabs, cluster));
  if (tab === null) {
    return <span className="text-fg-faint">unknown</span>;
  }
  return (
    <span className="flex items-center gap-1.5 truncate text-fg-muted">
      <span
        aria-hidden="true"
        style={{ backgroundColor: colorVar(tab.color) }}
        className="h-2 w-2 shrink-0 rounded-sm"
      />
      <span className="truncate">{nameOf(tab)}</span>
    </span>
  );
}

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
  const [fleet, setFleet] = useState(false);
  const [older, setOlder] = useState<HistoryEntry[]>([]);
  const [reaching, setReaching] = useState(false);
  const several = useTabStrip();
  const showing = fleet && several;
  const { data, error, reload } = useHistory({ source, fleet: showing });
  const held = useMemory();
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

  const page = data;

  async function reachBack() {
    setReaching(true);
    try {
      const next = await fetchHistory({ source, fleet: showing, after: cursorOf(page, older) });
      setOlder((have) => [...have, ...next.entries]);
    } catch (err: unknown) {
      notifyError(olderFailure(err));
    } finally {
      setReaching(false);
    }
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
        {several && (
          <label className="flex items-center gap-1.5 text-fg-soft">
            <input
              type="checkbox"
              checked={fleet}
              onChange={(event) => {
                setFleet(event.target.checked);
              }}
            />
            Every open cluster
          </label>
        )}
        <Recording />
        {data.more && older.length === 0 && (
          <span className="text-fg-muted">showing the newest {HISTORY_LIMIT}</span>
        )}
        {held.data !== null && (
          <span className="text-fg-muted" title="what spinoza is holding while it watches">
            {held.data.heapMi} MB held
          </span>
        )}
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
                {HEADERS.filter((header) => header.fleet !== true || showing).map((header) => (
                  <th key={header.id} className={`px-2 py-1 font-medium ${header.width}`}>
                    {header.label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {foldRepeats([...data.entries, ...older]).map((folded) => (
                <Row
                  key={`${folded.entry.source}-${String(folded.entry.id)}`}
                  folded={folded}
                  now={now}
                  fleet={showing}
                  onOpen={onOpen}
                />
              ))}
            </tbody>
          </table>
          {reachable(data, older) && (
            <button
              type="button"
              disabled={reaching}
              onClick={() => {
                void reachBack();
              }}
              className={`${CONTROL} m-2 border-edge-strong text-fg-soft hover:bg-surface-active disabled:opacity-50`}
            >
              {reaching ? 'Reaching back…' : 'Show older'}
            </button>
          )}
        </div>
      )}
    </div>
  );
}

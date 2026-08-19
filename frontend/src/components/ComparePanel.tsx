import { Suspense, lazy, useMemo, useState } from 'react';
import type { Comparison, ObjectRef } from '../lib/types';
import type { CompareTarget } from '../lib/compare';
import { changedSections, differingLines, fetchComparison } from '../lib/compare';
import { contextGroups, sameContext } from '../lib/contexts';
import type { ContextEntry } from '../lib/contexts';
import { useContextList } from '../store/contexts';
import { useElementWidth } from '../lib/useElementWidth';
import Announce from './Announce';
import Loading from './Loading';

const YamlDiff = lazy(() => import('./YamlDiff'));

interface ComparePanelProps {
  target: ObjectRef | null;
}

const SIDE_BY_SIDE_FROM = 900;

function summary(result: Comparison): string {
  if (result.identical) {
    return 'identical once the per-cluster fields are stripped';
  }
  const lines = differingLines(result.left, result.right);
  const sections = changedSections(result.left, result.right);
  if (sections.length === 0) {
    return `${String(lines)} lines differ`;
  }
  return `${String(lines)} lines differ · ${sections.join(', ')}`;
}

function reason(err: unknown): string {
  if (err instanceof Error && err.message !== '') {
    return err.message;
  }
  return 'the comparison did not run';
}

export default function ComparePanel({ target }: ComparePanelProps) {
  const list = useContextList();

  const others = useMemo(() => {
    const entries = contextGroups(list).flatMap((group) => group.entries);
    return entries.filter((entry) => !sameContext(entry, list.current));
  }, [list]);

  if (target === null) {
    return <p className="p-4 text-xs text-fg-muted">Select an object to compare it.</p>;
  }

  if (others.length === 0) {
    return (
      <p className="p-4 text-xs text-fg-muted">
        Comparing needs a second context. Add a kubeconfig, or switch to one that has more.
      </p>
    );
  }

  const identity = `${target.resource}/${target.namespace}/${target.name}`;
  return <Comparing key={identity} target={target} others={others} />;
}

interface ComparingProps {
  target: ObjectRef;
  others: ContextEntry[];
}

function Comparing({ target, others }: ComparingProps) {
  const [chosen, setChosen] = useState('');
  const [namespace, setNamespace] = useState(target.namespace);
  const [raw, setRaw] = useState(false);
  const [result, setResult] = useState<Comparison | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [host, setHost] = useState<HTMLDivElement | null>(null);
  const width = useElementWidth(host);

  const picked = others.find((entry) => entry.value === chosen) ?? null;

  async function run(against: ContextEntry) {
    setBusy(true);
    setError(null);
    const wanted: CompareTarget = {
      kubeconfig: against.kubeconfig,
      name: against.name,
      namespace,
      object: target.name,
    };
    try {
      setResult(await fetchComparison(target, wanted, raw));
    } catch (err: unknown) {
      setResult(null);
      setError(reason(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div ref={setHost} className="flex min-h-0 flex-1 flex-col">
      <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-edge px-3 py-2 text-xs">
        <label htmlFor="compare-context" className="text-fg-muted">
          Against
        </label>
        <select
          id="compare-context"
          value={chosen}
          onChange={(event) => {
            setChosen(event.target.value);
          }}
          className="rounded border border-edge-strong bg-surface-raised px-2 py-1 text-fg"
        >
          <option value="">Pick a context</option>
          {others.map((entry) => (
            <option key={entry.value} value={entry.value}>
              {entry.name}
            </option>
          ))}
        </select>
        <label htmlFor="compare-namespace" className="text-fg-muted">
          Namespace
        </label>
        <input
          id="compare-namespace"
          value={namespace}
          onChange={(event) => {
            setNamespace(event.target.value);
          }}
          className="w-40 rounded border border-edge-strong bg-surface-raised px-2 py-1 font-mono text-fg"
        />
        <button
          type="button"
          disabled={picked === null || busy}
          onClick={() => {
            if (picked !== null) {
              void run(picked);
            }
          }}
          className="rounded border border-edge-strong px-2 py-1 text-fg hover:bg-surface-active disabled:cursor-not-allowed disabled:text-fg-faint"
        >
          Compare
        </button>
        <label className="flex items-center gap-1 text-fg-muted">
          <input
            type="checkbox"
            checked={raw}
            onChange={(event) => {
              setRaw(event.target.checked);
            }}
          />
          Show everything
        </label>
        {busy && <span className="text-fg-muted">reading</span>}
      </div>
      <Announce message={error} urgent className="px-3 py-1.5 text-xs break-words text-error" />
      {result?.missing !== undefined && (
        <p className="px-3 py-2 text-xs text-warn">{result.missing}</p>
      )}
      {result !== null && result.missing === undefined && (
        <>
          <p className="px-3 py-1.5 text-xs text-fg-muted">
            {result.leftContext} against {result.rightContext} · {summary(result)}
          </p>
          <div className="min-h-0 flex-1">
            <Suspense fallback={<Loading what="the diff" />}>
              <YamlDiff
                left={result.left}
                right={result.right}
                sideBySide={width >= SIDE_BY_SIDE_FROM}
              />
            </Suspense>
          </div>
        </>
      )}
    </div>
  );
}

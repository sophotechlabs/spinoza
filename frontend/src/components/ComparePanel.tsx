import { Suspense, lazy, useMemo, useRef, useState } from 'react';
import type { Comparison, ObjectRef, ResourceDescriptor } from '../lib/types';
import type { CompareTarget } from '../lib/compare';
import { changedSections, differingLines, fetchComparison } from '../lib/compare';
import { contextGroups, sameContext } from '../lib/contexts';
import type { ContextEntry } from '../lib/contexts';
import { useContextList } from '../store/contexts';
import { useElementWidth } from '../lib/useElementWidth';
import Announce from './Announce';
import Loading from './Loading';

const YamlDiff = lazy(() => import('./YamlDiff'));
const KindCompare = lazy(() => import('./KindCompare'));

interface ComparePanelProps {
  target: ObjectRef | null;
  kind: ResourceDescriptor | null;
  namespace: string;
  onOpen: (ref: ObjectRef) => void;
}

type Scope = 'object' | 'kind';

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

export default function ComparePanel({ target, kind, namespace, onOpen }: ComparePanelProps) {
  const list = useContextList();
  const [chosen, setChosen] = useState('');

  const others = useMemo(() => {
    const entries = contextGroups(list).flatMap((group) => group.entries);
    return entries.filter((entry) => !sameContext(entry, list.current));
  }, [list]);

  if (target === null && kind === null) {
    return (
      <p className="p-4 text-xs text-fg-muted">Open a kind, or select an object, to compare.</p>
    );
  }

  if (others.length === 0) {
    return (
      <p className="p-4 text-xs text-fg-muted">
        Comparing needs a second context. Add a kubeconfig, or switch to one that has more.
      </p>
    );
  }

  const identity = JSON.stringify({
    current: list.current,
    kind,
    namespace,
    others: others.map((entry) => ({ kubeconfig: entry.kubeconfig, name: entry.name })),
    target,
  });
  return (
    <Comparing
      key={identity}
      target={target}
      kind={kind}
      scope={namespace}
      others={others}
      chosen={chosen}
      onChoose={setChosen}
      onOpen={onOpen}
    />
  );
}

interface ComparingProps {
  target: ObjectRef | null;
  kind: ResourceDescriptor | null;
  scope: string;
  others: ContextEntry[];
  chosen: string;
  onChoose: (value: string) => void;
  onOpen: (ref: ObjectRef) => void;
}

function Comparing({ target, kind, scope, others, chosen, onChoose, onOpen }: ComparingProps) {
  const [where, setWhere] = useState<Scope>(opensOn(target));
  const [namespace, setNamespace] = useState(target?.namespace ?? scope);
  const [raw, setRaw] = useState(false);
  const [result, setResult] = useState<Comparison | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [host, setHost] = useState<HTMLDivElement | null>(null);
  const asked = useRef(0);
  const width = useElementWidth(host);

  const picked = others.find((entry) => entry.value === chosen) ?? null;

  function invalidate() {
    asked.current += 1;
    setResult(null);
    setError(null);
    setBusy(false);
  }

  async function run(against: ContextEntry, object: ObjectRef) {
    const token = asked.current + 1;
    asked.current = token;
    setBusy(true);
    setError(null);
    const wanted: CompareTarget = {
      kubeconfig: against.kubeconfig,
      name: against.name,
      namespace,
      object: object.name,
    };
    try {
      const found = await fetchComparison(object, wanted, raw);
      if (asked.current !== token) {
        return;
      }
      setResult(found);
    } catch (err: unknown) {
      if (asked.current !== token) {
        return;
      }
      setResult(null);
      setError(reason(err));
    } finally {
      if (asked.current === token) {
        setBusy(false);
      }
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
            invalidate();
            onChoose(event.target.value);
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
        <label htmlFor="compare-scope" className="text-fg-muted">
          Compare
        </label>
        <select
          id="compare-scope"
          aria-label="What to compare"
          value={where}
          onChange={(event) => {
            invalidate();
            setWhere(event.target.value as Scope);
          }}
          className="rounded border border-edge-strong bg-surface-raised px-2 py-1 text-fg"
        >
          <option value="object" disabled={target === null}>
            this object
          </option>
          <option value="kind" disabled={kind === null}>
            every {kind?.kind ?? 'object'}
          </option>
        </select>
        <label htmlFor="compare-namespace" className="text-fg-muted">
          Namespace
        </label>
        <input
          id="compare-namespace"
          value={namespace}
          onChange={(event) => {
            invalidate();
            setNamespace(event.target.value);
          }}
          className="w-40 rounded border border-edge-strong bg-surface-raised px-2 py-1 font-mono text-fg"
        />
        {where === 'object' && (
          <>
            <button
              type="button"
              disabled={picked === null || target === null || busy}
              onClick={() => {
                if (picked !== null && target !== null) {
                  void run(picked, target);
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
                  invalidate();
                  setRaw(event.target.checked);
                }}
              />
              Show everything
            </label>
          </>
        )}
        {busy && <span className="text-fg-muted">reading</span>}
      </div>
      {where === 'kind' && kind !== null && picked === null && (
        <p className="px-3 py-2 text-xs text-fg-muted">Pick a context to compare against.</p>
      )}
      {where === 'kind' && kind !== null && picked !== null && (
        <Suspense fallback={<Loading what="the comparison" />}>
          <KindCompare
            key={`${picked.value}/${namespace}`}
            kind={kind}
            namespace={namespace}
            target={{
              kubeconfig: picked.kubeconfig,
              name: picked.name,
              namespace,
              object: '',
            }}
            onOpen={onOpen}
          />
        </Suspense>
      )}
      {where === 'object' && (
        <>
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
        </>
      )}
    </div>
  );
}

function opensOn(target: ObjectRef | null): Scope {
  if (target === null) {
    return 'kind';
  }
  return 'object';
}

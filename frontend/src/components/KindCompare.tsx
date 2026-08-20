import { useState } from 'react';
import type {
  KindComparison,
  KindDiff,
  ObjectRef,
  ResourceDescriptor,
  Verdict,
} from '../lib/types';
import type { CompareTarget } from '../lib/compare';
import { fetchKindComparison, summaryOf } from '../lib/compare';

interface KindCompareProps {
  kind: ResourceDescriptor;
  namespace: string;
  target: CompareTarget;
  onOpen: (ref: ObjectRef) => void;
}

const VERDICT_LABELS: Record<Verdict, string> = {
  same: 'same',
  differs: 'differs',
  onlyHere: 'only here',
  onlyThere: 'only there',
};

function verdictColor(verdict: Verdict): string {
  if (verdict === 'same') {
    return 'text-ok';
  }
  if (verdict === 'differs') {
    return 'text-warn';
  }
  return 'text-fg-muted';
}

function detail(object: KindDiff): string {
  if (object.verdict !== 'differs') {
    return '';
  }
  if (object.lines === undefined || object.lines === 0) {
    return '';
  }
  return `${String(object.lines)} lines`;
}

export default function KindCompare({ kind, namespace, target, onOpen }: KindCompareProps) {
  const [result, setResult] = useState<KindComparison | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [driftOnly, setDriftOnly] = useState(true);

  async function run() {
    setBusy(true);
    setError(null);
    try {
      setResult(await fetchKindComparison(kind, namespace, target));
    } catch (err: unknown) {
      setResult(null);
      setError(err instanceof Error ? err.message : 'the comparison did not run');
    } finally {
      setBusy(false);
    }
  }

  const shown = visible(result, driftOnly);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex shrink-0 flex-wrap items-center gap-2 px-3 py-1.5 text-xs">
        <button
          type="button"
          disabled={busy}
          onClick={() => {
            void run();
          }}
          className="rounded border border-edge-strong px-2 py-1 text-fg hover:bg-surface-active disabled:cursor-not-allowed disabled:text-fg-faint"
        >
          Compare every {kind.kind}
        </button>
        <label className="flex items-center gap-1 text-fg-muted">
          <input
            type="checkbox"
            checked={driftOnly}
            onChange={(event) => {
              setDriftOnly(event.target.checked);
            }}
          />
          Only what differs
        </label>
        {busy && <span className="text-fg-muted">reading both clusters</span>}
      </div>
      {error !== null && (
        <p role="alert" className="px-3 py-1.5 text-xs break-words text-error">
          {error}
        </p>
      )}
      {result !== null && (
        <>
          <p className="px-3 py-1.5 text-xs text-fg-muted">
            {result.leftContext} against {result.rightContext} · {summaryOf(result)}
            {result.matchedByName === true && ' · matched by name across namespaces'}
          </p>
          {shown.length === 0 && (
            <p className="px-3 py-2 text-xs text-fg-muted">{emptyNote(result)}</p>
          )}
          {shown.length > 0 && (
            <div className="min-h-0 flex-1 overflow-auto">
              <table className="w-full border-collapse text-left text-xs whitespace-nowrap">
                <thead className="sticky top-0 z-10 bg-surface-raised text-fg-muted">
                  <tr className="border-b border-edge">
                    <th className="px-2 py-1 font-medium">Name</th>
                    <th className="px-2 py-1 font-medium">Namespace</th>
                    <th className="px-2 py-1 font-medium">Verdict</th>
                    <th className="px-2 py-1 font-medium">Detail</th>
                  </tr>
                </thead>
                <tbody>
                  {shown.map((object) => (
                    <tr
                      key={`${object.namespace ?? ''}/${object.name}`}
                      className="border-b border-edge last:border-b-0 hover:bg-surface-raised"
                    >
                      <td className="px-2 py-1 text-fg-strong">
                        <button
                          type="button"
                          disabled={object.verdict === 'onlyThere'}
                          onClick={() => {
                            onOpen(refFor(kind, object, namespace));
                          }}
                          className="max-w-full truncate hover:underline disabled:cursor-not-allowed disabled:no-underline"
                        >
                          {object.name}
                        </button>
                      </td>
                      <td className="px-2 py-1 text-fg-muted">{object.namespace ?? ''}</td>
                      <td className={`px-2 py-1 ${verdictColor(object.verdict)}`}>
                        {VERDICT_LABELS[object.verdict]}
                      </td>
                      <td className="px-2 py-1 text-fg-muted">{detail(object)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}
    </div>
  );
}

function visible(result: KindComparison | null, driftOnly: boolean): KindDiff[] {
  if (result === null) {
    return [];
  }
  if (!driftOnly) {
    return result.objects;
  }
  return result.objects.filter((object) => object.verdict !== 'same');
}

// Only reached when nothing is on screen, which means either the two clusters
// hold none of this kind at all, or every one of them matched and the drift
// filter took them away.
function emptyNote(result: KindComparison): string {
  if (result.objects.length === 0) {
    return 'Neither cluster has one of these.';
  }
  return 'Every one of them matches.';
}

function refFor(kind: ResourceDescriptor, object: KindDiff, namespace: string): ObjectRef {
  return {
    group: kind.group,
    version: kind.version,
    resource: kind.resource,
    namespace: object.namespace ?? namespace,
    name: object.name,
  };
}

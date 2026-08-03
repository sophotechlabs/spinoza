import { useEffect, useState } from 'react';
import type { ActionResult, ObjectDetail, ObjectRef, PodOutcome } from '../lib/types';
import {
  canRestart,
  canScale,
  countBy,
  isCordoned,
  isNode,
  replicasOf,
  runAction,
} from '../lib/objectActions';
import type { ObjectAction } from '../lib/objectActions';
import { refQuery } from '../lib/object';

interface InspectObjectActionsProps {
  target: ObjectRef;
  detail: ObjectDetail | null;
  onDone: () => void;
}

const buttonClass =
  'rounded border border-neutral-700 px-2 py-1 text-neutral-200 hover:bg-neutral-800 disabled:cursor-not-allowed disabled:text-neutral-600';

const dangerClass =
  'rounded border border-amber-900 px-2 py-1 text-amber-400 hover:bg-amber-950 disabled:cursor-not-allowed disabled:text-neutral-600';

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'action failed';
}

function outcomeClass(outcome: string): string {
  if (outcome === 'blocked') {
    return 'text-amber-400';
  }
  if (outcome === 'failed') {
    return 'text-red-400';
  }
  if (outcome === 'skipped') {
    return 'text-neutral-400';
  }
  return 'text-green-400';
}

function PodList({ pods }: { pods: PodOutcome[] }) {
  return (
    <ul className="mt-1.5 max-h-40 overflow-y-auto">
      {pods.map((pod) => (
        <li key={`${pod.namespace}/${pod.name}`} className="flex gap-2 py-0.5">
          <span className={`w-14 shrink-0 ${outcomeClass(pod.outcome)}`}>{pod.outcome}</span>
          <span className="truncate text-neutral-300">{pod.name}</span>
          {pod.reason !== undefined && (
            <span className="truncate text-neutral-400">{pod.reason}</span>
          )}
        </li>
      ))}
    </ul>
  );
}

export default function InspectObjectActions({
  target,
  detail,
  onDone,
}: InspectObjectActionsProps) {
  const [replicas, setReplicas] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [plan, setPlan] = useState<ActionResult | null>(null);
  const [force, setForce] = useState(false);

  const refKey = refQuery(target);

  useEffect(() => {
    setError(null);
    setNotice(null);
    setPlan(null);
    setForce(false);
  }, [refKey]);

  const currentReplicas = replicasOf(detail);

  useEffect(() => {
    setReplicas(String(currentReplicas));
  }, [currentReplicas]);

  async function run(action: ObjectAction, options: Record<string, unknown> = {}) {
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      const result = await runAction(target, action, options);
      if (result.dryRun === true) {
        setPlan(result);
      } else {
        setPlan(null);
        setNotice(result.message);
      }
      onDone();
      return result;
    } catch (err: unknown) {
      setError(errorMessage(err));
      setPlan(null);
      return null;
    } finally {
      setBusy(false);
    }
  }

  function handleScale() {
    if (replicas.trim() === '') {
      setError('replicas must be a whole number, zero or more');
      return;
    }
    const count = Number(replicas);
    if (!Number.isInteger(count) || count < 0) {
      setError('replicas must be a whole number, zero or more');
      return;
    }
    void run('scale', { replicas: count });
  }

  const blocked = plan === null ? 0 : countBy(plan, 'blocked');
  const cordoned = isCordoned(detail);
  const confirmDisabled = busy || (blocked > 0 && !force);

  return (
    <div className="shrink-0 border-b border-neutral-800 px-3 py-2 text-xs">
      <div className="flex flex-wrap items-center gap-2">
        {canScale(target) && (
          <>
            <label className="text-neutral-400" htmlFor="replica-count">
              replicas
            </label>
            <input
              id="replica-count"
              type="number"
              min={0}
              value={replicas}
              onChange={(event) => {
                setReplicas(event.target.value);
              }}
              className="w-16 rounded border border-neutral-700 bg-neutral-900 px-1 py-0.5 text-neutral-200"
            />
            <button type="button" onClick={handleScale} disabled={busy} className={buttonClass}>
              Scale
            </button>
          </>
        )}
        {canRestart(target) && (
          <button
            type="button"
            onClick={() => void run('restart')}
            disabled={busy}
            className={buttonClass}
          >
            Restart
          </button>
        )}
        {isNode(target) && !cordoned && (
          <button
            type="button"
            onClick={() => void run('cordon')}
            disabled={busy}
            className={dangerClass}
          >
            Cordon
          </button>
        )}
        {isNode(target) && cordoned && (
          <button
            type="button"
            onClick={() => void run('uncordon')}
            disabled={busy}
            className="rounded border border-green-900 px-2 py-1 text-green-400 hover:bg-green-950 disabled:cursor-not-allowed disabled:text-neutral-600"
          >
            Uncordon
          </button>
        )}
        {isNode(target) && (
          <button
            type="button"
            onClick={() => void run('drain', { dryRun: true })}
            disabled={busy}
            className={dangerClass}
          >
            Drain
          </button>
        )}
        {cordoned && <span className="text-amber-500">cordoned</span>}
        {busy && <span className="text-neutral-400">working…</span>}
      </div>

      {plan !== null && (
        <div className="mt-2 rounded border border-neutral-800 bg-neutral-900/60 p-2">
          <p className="text-neutral-300">{plan.message}</p>
          <PodList pods={plan.pods ?? []} />
          {blocked > 0 && (
            <label className="mt-1.5 flex items-center gap-1.5 text-amber-400">
              <input
                type="checkbox"
                checked={force}
                onChange={(event) => {
                  setForce(event.target.checked);
                }}
              />
              Evict the blocked pods anyway
            </label>
          )}
          <div className="mt-2 flex gap-2">
            <button
              type="button"
              onClick={() => void run('drain', { force })}
              disabled={confirmDisabled}
              className={dangerClass}
            >
              Drain now
            </button>
            <button
              type="button"
              onClick={() => {
                setPlan(null);
              }}
              disabled={busy}
              className={buttonClass}
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {error !== null && <p className="mt-1.5 break-words text-red-400">{error}</p>}
      {notice !== null && <p className="mt-1.5 break-words text-green-400">{notice}</p>}
    </div>
  );
}

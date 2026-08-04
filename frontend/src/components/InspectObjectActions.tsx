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
  'rounded border border-edge-strong px-2 py-1 text-fg hover:bg-surface-active disabled:cursor-not-allowed disabled:text-fg-faint';

const dangerClass =
  'rounded border border-warn-line px-2 py-1 text-warn hover:bg-warn-tint disabled:cursor-not-allowed disabled:text-fg-faint';

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'action failed';
}

function outcomeClass(outcome: string): string {
  if (outcome === 'blocked') {
    return 'text-warn';
  }
  if (outcome === 'failed') {
    return 'text-error';
  }
  if (outcome === 'skipped') {
    return 'text-fg-muted';
  }
  return 'text-ok';
}

function PodList({ pods }: { pods: PodOutcome[] }) {
  return (
    <ul className="mt-1.5 max-h-40 overflow-y-auto">
      {pods.map((pod) => (
        <li key={`${pod.namespace}/${pod.name}`} className="flex gap-2 py-0.5">
          <span className={`w-14 shrink-0 ${outcomeClass(pod.outcome)}`}>{pod.outcome}</span>
          <span className="truncate text-fg-soft">{pod.name}</span>
          {pod.reason !== undefined && <span className="truncate text-fg-muted">{pod.reason}</span>}
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
    <div className="shrink-0 border-b border-edge px-3 py-2 text-xs">
      <div className="flex flex-wrap items-center gap-2">
        {canScale(target) && (
          <>
            <label className="text-fg-muted" htmlFor="replica-count">
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
              className="w-16 rounded border border-edge-strong bg-surface-raised px-1 py-0.5 text-fg"
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
            className="rounded border border-ok-line px-2 py-1 text-ok hover:bg-ok-tint disabled:cursor-not-allowed disabled:text-fg-faint"
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
        {cordoned && <span className="text-warn-muted">cordoned</span>}
        {busy && <span className="text-fg-muted">working…</span>}
      </div>

      {plan !== null && (
        <div className="mt-2 rounded border border-edge bg-surface-raised/60 p-2">
          <p className="text-fg-soft">{plan.message}</p>
          <PodList pods={plan.pods ?? []} />
          {blocked > 0 && (
            <label className="mt-1.5 flex items-center gap-1.5 text-warn">
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

      {error !== null && <p className="mt-1.5 break-words text-error">{error}</p>}
      {notice !== null && <p className="mt-1.5 break-words text-ok">{notice}</p>}
    </div>
  );
}

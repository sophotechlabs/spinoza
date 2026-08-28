import { useEffect, useState } from 'react';
import type { ActionResult, ObjectDetail, ObjectRef, PodOutcome } from '../lib/types';
import {
  canRestart,
  canScale,
  countBy,
  isCordoned,
  isCronJob,
  isNode,
  isSuspended,
  replicasOf,
  runAction,
} from '../lib/objectActions';
import type { ObjectAction } from '../lib/objectActions';
import { refQuery } from '../lib/object';
import { notifyError, notifyOk } from '../store/toasts';
import Announce from './Announce';
import ConfirmByName from './ConfirmByName';
import NodeShellButton from './NodeShellButton';
import { useProtectedCluster } from '../store/contexts';
import { useRefusal } from '../store/access';

interface InspectObjectActionsProps {
  target: ObjectRef;
  detail: ObjectDetail | null;
  onDone: () => void;
}

const buttonClass =
  'rounded border border-edge-strong px-2 py-1 text-fg hover:bg-surface-active disabled:cursor-not-allowed disabled:text-fg-faint';

const dangerClass =
  'rounded border border-warn-line px-2 py-1 text-warn hover:bg-warn-tint disabled:cursor-not-allowed disabled:text-fg-faint';

const resumeClass =
  'rounded border border-ok-line px-2 py-1 text-ok hover:bg-ok-tint disabled:cursor-not-allowed disabled:text-fg-faint';

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'action failed';
}

interface Pending {
  action: ObjectAction;
  options: Record<string, unknown>;
  question: string;
  typed: boolean;
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
  const [pending, setPending] = useState<Pending | null>(null);
  const protectedCluster = useProtectedCluster();
  const noScale = useRefusal(target, 'scale');
  const noRestart = useRefusal(target, 'restart');
  const noCordon = useRefusal(target, 'cordon');
  const noDrain = useRefusal(target, 'drain');
  const noSuspend = useRefusal(target, 'suspend');
  const noTrigger = useRefusal(target, 'trigger');

  const refKey = refQuery(target);

  useEffect(() => {
    setError(null);
    setNotice(null);
    setPlan(null);
    setForce(false);
    setPending(null);
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
        notifyOk(`${target.name}: ${result.message}`, target);
      }
      onDone();
      return result;
    } catch (err: unknown) {
      const message = errorMessage(err);
      setError(message);
      notifyError(`${action} ${target.name}: ${message}`, target);
      setPlan(null);
      return null;
    } finally {
      setBusy(false);
    }
  }

  function ask(action: ObjectAction, options: Record<string, unknown>, question: string) {
    setError(null);
    setNotice(null);
    setPending({ action, options, question, typed: false });
  }

  function askTyped(action: ObjectAction, options: Record<string, unknown>, question: string) {
    setError(null);
    setNotice(null);
    setPending({ action, options, question, typed: true });
  }

  function confirmPending(chosen: Pending) {
    setPending(null);
    if (chosen.typed) {
      void run(chosen.action, { ...chosen.options, confirm: target.name });
      return;
    }
    void run(chosen.action, chosen.options);
  }

  function cancelPending() {
    setPending(null);
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
    if (count === 0) {
      const question = `Scale ${target.name} to zero? Every pod is removed.`;
      if (protectedCluster) {
        askTyped('scale', { replicas: 0 }, question);
        return;
      }
      ask('scale', { replicas: 0 }, question);
      return;
    }
    void run('scale', { replicas: count });
  }

  function drainNow() {
    if (protectedCluster) {
      askTyped('drain', { force }, `Draining node ${target.name} evicts its pods.`);
      return;
    }
    void run('drain', { force });
  }

  const blocked = plan === null ? 0 : countBy(plan, 'blocked');
  const cordoned = isCordoned(detail);
  const suspended = isSuspended(detail);
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
            <button
              type="button"
              onClick={handleScale}
              disabled={busy || noScale !== null}
              title={noScale ?? undefined}
              className={buttonClass}
            >
              Scale
            </button>
          </>
        )}
        {canRestart(target) && (
          <button
            type="button"
            onClick={() => {
              ask('restart', {}, `Restart ${target.name}? Every pod is replaced.`);
            }}
            disabled={busy || noRestart !== null}
            title={noRestart ?? undefined}
            className={buttonClass}
          >
            Restart
          </button>
        )}
        {isNode(target) && !cordoned && (
          <button
            type="button"
            onClick={() => {
              ask('cordon', {}, `Cordon ${target.name}? Nothing new will be scheduled on it.`);
            }}
            disabled={busy || noCordon !== null}
            title={noCordon ?? undefined}
            className={dangerClass}
          >
            Cordon
          </button>
        )}
        {isNode(target) && cordoned && (
          <button
            type="button"
            onClick={() => void run('uncordon')}
            disabled={busy || noCordon !== null}
            title={noCordon ?? undefined}
            className={resumeClass}
          >
            Uncordon
          </button>
        )}
        {isNode(target) && (
          <button
            type="button"
            onClick={() => void run('drain', { dryRun: true })}
            disabled={busy || noDrain !== null}
            title={noDrain ?? undefined}
            className={dangerClass}
          >
            Drain
          </button>
        )}
        {isCronJob(target) && (
          <button
            type="button"
            onClick={() => {
              ask('trigger', {}, `Run ${target.name} now? A job is started outside the schedule.`);
            }}
            disabled={busy || noTrigger !== null}
            title={noTrigger ?? undefined}
            className={buttonClass}
          >
            Run now
          </button>
        )}
        {isCronJob(target) && !suspended && (
          <button
            type="button"
            onClick={() => {
              ask('suspend', {}, `Suspend ${target.name}? No new runs are started.`);
            }}
            disabled={busy || noSuspend !== null}
            title={noSuspend ?? undefined}
            className={dangerClass}
          >
            Suspend
          </button>
        )}
        {isCronJob(target) && suspended && (
          <button
            type="button"
            onClick={() => void run('resume')}
            disabled={busy || noSuspend !== null}
            title={noSuspend ?? undefined}
            className={resumeClass}
          >
            Resume
          </button>
        )}
        {isNode(target) && <NodeShellButton node={target.name} />}
        {cordoned && <span className="text-warn-muted">cordoned</span>}
        {suspended && <span className="text-warn-muted">suspended</span>}
        {busy && <span className="text-fg-muted">working</span>}
      </div>

      {pending !== null && pending.typed && (
        <ConfirmByName
          open
          name={target.name}
          what={pending.question}
          onConfirm={() => {
            confirmPending(pending);
          }}
          onCancel={cancelPending}
        />
      )}
      {pending !== null && !pending.typed && (
        <div className="mt-2 flex flex-wrap items-center gap-2 rounded border border-warn-line bg-warn-tint/40 p-2">
          <span className="text-warn-strong">{pending.question}</span>
          <button
            type="button"
            onClick={() => {
              confirmPending(pending);
            }}
            disabled={busy}
            className={dangerClass}
          >
            Confirm
          </button>
          <button type="button" onClick={cancelPending} disabled={busy} className={buttonClass}>
            Cancel
          </button>
        </div>
      )}

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
              onClick={() => {
                drainNow();
              }}
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

      <Announce message={error} urgent className="mt-1.5 break-words text-error" />
      <Announce message={notice} className="mt-1.5 break-words text-ok" />
    </div>
  );
}

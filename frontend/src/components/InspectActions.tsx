import { useEffect, useRef, useState } from 'react';
import type { ObjectRef } from '../lib/types';
import { pollReconcile, runFluxAction } from '../lib/fluxActions';
import type { FluxAction, ReconcileProgress, ReconcileState } from '../lib/fluxActions';

interface InspectActionsProps {
  target: ObjectRef;
  suspended?: boolean;
  onDone: () => void;
}

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'action failed';
}

function noticeFor(action: FluxAction): string {
  if (action === 'reconcile') {
    return 'Reconciliation requested…';
  }
  if (action === 'suspend') {
    return 'Suspended.';
  }
  return 'Resumed.';
}

function noticeClass(state: ReconcileState | null): string {
  if (state === 'failed') {
    return 'mt-1.5 break-words text-red-400';
  }
  if (state === 'requested' || state === 'running') {
    return 'mt-1.5 break-words text-neutral-400';
  }
  return 'mt-1.5 break-words text-green-400';
}

export default function InspectActions({ target, suspended, onDone }: InspectActionsProps) {
  const [busy, setBusy] = useState<FluxAction | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [state, setState] = useState<ReconcileState | null>(null);
  const watchRef = useRef(0);

  useEffect(() => {
    setError(null);
    setNotice(null);
    setState(null);
    watchRef.current += 1;
  }, [target]);

  useEffect(() => {
    return () => {
      watchRef.current += 1;
    };
  }, []);

  async function run(action: FluxAction) {
    setBusy(action);
    setError(null);
    setNotice(null);
    setState(null);
    watchRef.current += 1;
    try {
      const result = await runFluxAction(target, action);
      setNotice(noticeFor(action));
      onDone();
      if (action === 'reconcile' && result.requestedAt !== undefined) {
        void watch(result.requestedAt);
      }
    } catch (err: unknown) {
      setError(errorMessage(err));
    } finally {
      setBusy(null);
    }
  }

  async function watch(requestedAt: string) {
    const token = watchRef.current;
    setState('requested');
    await pollReconcile(target, requestedAt, (progress: ReconcileProgress) => {
      if (watchRef.current !== token) {
        return false;
      }
      setState(progress.state);
      setNotice(progress.message);
      return true;
    });
    if (watchRef.current === token) {
      onDone();
    }
  }

  const disabled = busy !== null;

  return (
    <div className="shrink-0 border-b border-neutral-800 px-3 py-2 text-xs">
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={() => void run('reconcile')}
          disabled={disabled}
          className="rounded border border-neutral-700 px-2 py-1 text-neutral-200 hover:bg-neutral-800 disabled:cursor-not-allowed disabled:text-neutral-600"
        >
          Reconcile
        </button>
        {suspended === true && (
          <button
            type="button"
            onClick={() => void run('resume')}
            disabled={disabled}
            className="rounded border border-green-900 px-2 py-1 text-green-400 hover:bg-green-950 disabled:cursor-not-allowed disabled:text-neutral-600"
          >
            Resume
          </button>
        )}
        {suspended !== true && (
          <button
            type="button"
            onClick={() => void run('suspend')}
            disabled={disabled}
            className="rounded border border-amber-900 px-2 py-1 text-amber-400 hover:bg-amber-950 disabled:cursor-not-allowed disabled:text-neutral-600"
          >
            Suspend
          </button>
        )}
        {suspended === true && <span className="text-amber-500">suspended</span>}
        {busy !== null && <span className="text-neutral-500">working…</span>}
      </div>
      {error !== null && <p className="mt-1.5 break-words text-red-400">{error}</p>}
      {notice !== null && <p className={noticeClass(state)}>{notice}</p>}
    </div>
  );
}

import { useEffect, useId, useRef, useState } from 'react';
import type { ObjectRef } from '../lib/types';
import { pollReconcile, runFluxAction } from '../lib/fluxActions';
import type { FluxAction, ReconcileProgress, ReconcileState } from '../lib/fluxActions';
import { refQuery } from '../lib/object';
import Announce from './Announce';
import { useRefusal } from '../store/access';
import { useClusterEpoch } from '../store/cluster';
import { useGitopsKeys } from '../lib/gitopsKeys';
import DisabledActionReasons from './DisabledActionReasons';
import ActionGroup, { Action, ActionNote } from './ActionGroup';
import { actionTitle, describedBy } from '../lib/actionAvailability';

interface InspectActionsProps {
  target: ObjectRef;
  suspended?: boolean;
  terminating?: boolean;
  sourced?: boolean;
  onDone: () => void;
}

const TERMINATING = 'this object is being deleted';

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'action failed';
}

function noticeFor(action: FluxAction): string {
  if (action === 'reconcile') {
    return 'Reconciliation requested';
  }
  if (action === 'reconcile-with-source') {
    return 'Source and object asked to reconcile';
  }
  if (action === 'suspend') {
    return 'Suspended.';
  }
  return 'Resumed.';
}

function noticeClass(state: ReconcileState | null): string {
  if (state === 'failed') {
    return 'mt-1.5 break-words text-error';
  }
  if (state === 'requested' || state === 'running') {
    return 'mt-1.5 break-words text-fg-muted';
  }
  return 'mt-1.5 break-words text-ok';
}

export default function InspectActions({
  target,
  suspended,
  terminating,
  sourced,
  onDone,
}: InspectActionsProps) {
  const [busy, setBusy] = useState<FluxAction | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [state, setState] = useState<ReconcileState | null>(null);
  const epoch = useClusterEpoch();
  const watchRef = useRef(0);
  const targetKey = `${epoch}|${refQuery(target)}`;
  const reasonPrefix = useId();
  const blockedReasonId = `${reasonPrefix}-gitops`;

  useEffect(() => {
    setBusy(null);
    setError(null);
    setNotice(null);
    setState(null);
    watchRef.current += 1;
  }, [targetKey]);

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
    const token = watchRef.current;
    try {
      const result = await runFluxAction(target, action);
      if (watchRef.current !== token) {
        return;
      }
      setNotice(noticeFor(action));
      onDone();
      if (action !== 'suspend' && action !== 'resume' && result.requestedAt !== undefined) {
        void watch(result.requestedAt, token);
      }
    } catch (err: unknown) {
      if (watchRef.current !== token) {
        return;
      }
      setError(errorMessage(err));
    } finally {
      if (watchRef.current === token) {
        setBusy(null);
      }
    }
  }

  async function watch(requestedAt: string, token: number) {
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

  const refused = useRefusal(target, 'reconcile');
  const disabled = busy !== null || refused !== null || terminating === true;
  let blocked = refused;
  if (terminating === true) {
    blocked = TERMINATING;
  }

  useGitopsKeys({
    sync: () => {
      if (disabled) {
        return;
      }
      void run('reconcile');
    },
    deepRefresh: () => {
      if (disabled || sourced !== true) {
        return;
      }
      void run('reconcile-with-source');
    },
  });

  return (
    <div className="shrink-0 border-b border-edge px-3 py-2 text-xs">
      <ActionGroup label="GitOps actions">
        <Action
          label="Reconcile"
          onClick={() => void run('reconcile')}
          disabled={disabled}
          describedBy={describedBy(blocked, blockedReasonId)}
          title={actionTitle(blocked)}
        />
        {sourced === true && (
          <Action
            label="With source"
            onClick={() => void run('reconcile-with-source')}
            disabled={disabled}
            describedBy={describedBy(blocked, blockedReasonId)}
            title={actionTitle(blocked, 'Ask the repository to fetch first, then reconcile this')}
          />
        )}
        {suspended === true && (
          <Action
            label="Resume"
            tone="good"
            onClick={() => void run('resume')}
            disabled={disabled}
            describedBy={describedBy(blocked, blockedReasonId)}
            title={actionTitle(blocked)}
          />
        )}
        {suspended !== true && (
          <Action
            label="Suspend"
            tone="caution"
            onClick={() => void run('suspend')}
            disabled={disabled}
            describedBy={describedBy(blocked, blockedReasonId)}
            title={actionTitle(blocked)}
          />
        )}
        {suspended === true && <span className="text-warn-muted">suspended</span>}
        {busy !== null && <ActionNote>working</ActionNote>}
      </ActionGroup>
      <DisabledActionReasons
        reasons={[{ id: blockedReasonId, label: 'GitOps actions', reason: blocked }]}
      />
      <Announce message={error} urgent className="mt-1.5 break-words text-error" />
      <Announce message={notice} className={noticeClass(state)} />
    </div>
  );
}

import { useEffect, useRef, useState } from 'react';
import type { ObjectRef } from '../lib/types';
import type { ArgoAction, ArgoOptions } from '../lib/argoActions';
import { runArgoAction } from '../lib/argoActions';
import { refQuery } from '../lib/object';
import { confirmName } from '../lib/contexts';
import { useProtectedCluster } from '../store/contexts';
import { useClusterEpoch } from '../store/cluster';
import { useRefusal } from '../store/access';
import { useGitopsKeys } from '../lib/gitopsKeys';
import Announce from './Announce';
import ArgoSyncDialog from './ArgoSyncDialog';
import ConfirmByName from './ConfirmByName';

interface ArgoActionsProps {
  target: ObjectRef;
  suspended?: boolean;
  terminating?: boolean;
  onDone: () => void;
}

const TERMINATING = 'this application is being deleted';

const NOTICES: Record<ArgoAction, string> = {
  sync: 'Sync requested.',
  refresh: 'Refresh requested.',
  'hard-refresh': 'Hard refresh requested.',
  terminate: 'Termination requested.',
  suspend: 'Auto-sync off. Prune and self-heal unchanged.',
  resume: 'Auto-sync on.',
  rollback: 'Rollback requested.',
};

const NEEDS_NAME: ArgoAction[] = ['sync', 'suspend', 'resume', 'rollback'];

const buttonClass =
  'rounded border border-edge-strong px-2 py-1 text-fg hover:bg-surface-active disabled:cursor-not-allowed disabled:text-fg-faint';

const warnClass =
  'rounded border border-warn-line px-2 py-1 text-warn hover:bg-warn-tint disabled:cursor-not-allowed disabled:text-fg-faint';

const okClass =
  'rounded border border-ok-line px-2 py-1 text-ok hover:bg-ok-tint disabled:cursor-not-allowed disabled:text-fg-faint';

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'action failed';
}

function what(action: ArgoAction, name: string): string {
  if (action === 'suspend') {
    return `Turning auto-sync off for Application ${name}.`;
  }
  if (action === 'resume') {
    return `Turning auto-sync back on for Application ${name}.`;
  }
  return `Syncing Application ${name} against its repository.`;
}

export default function ArgoActions({ target, suspended, terminating, onDone }: ArgoActionsProps) {
  const [busy, setBusy] = useState<ArgoAction | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [asking, setAsking] = useState<ArgoAction | null>(null);
  const [choosing, setChoosing] = useState(false);
  const [pending, setPending] = useState<ArgoOptions | undefined>(undefined);
  const protectedCluster = useProtectedCluster();
  const epoch = useClusterEpoch();
  const runRef = useRef(0);
  const targetKey = `${epoch}|${refQuery(target)}`;

  useEffect(() => {
    setBusy(null);
    setError(null);
    setNotice(null);
    setAsking(null);
    setChoosing(false);
    setPending(undefined);
    runRef.current += 1;
  }, [targetKey]);

  useEffect(() => {
    return () => {
      runRef.current += 1;
    };
  }, []);

  async function run(action: ArgoAction, options?: ArgoOptions) {
    setBusy(action);
    setError(null);
    setNotice(null);
    setAsking(null);
    runRef.current += 1;
    const token = runRef.current;
    try {
      await runArgoAction(target, action, confirmName(protectedCluster, target.name), options);
      if (runRef.current !== token) {
        return;
      }
      setNotice(NOTICES[action]);
      onDone();
    } catch (err: unknown) {
      if (runRef.current !== token) {
        return;
      }
      setError(errorMessage(err));
    } finally {
      if (runRef.current === token) {
        setBusy(null);
      }
    }
  }

  function ask(action: ArgoAction, options?: ArgoOptions) {
    if (protectedCluster && NEEDS_NAME.includes(action)) {
      setPending(options);
      setAsking(action);
      return;
    }
    void run(action, options);
  }

  const refused = useRefusal(target, 'reconcile');
  const disabled = busy !== null || refused !== null;
  let blocked = refused;
  if (terminating === true) {
    blocked = TERMINATING;
  }
  const writesDisabled = disabled || terminating === true;

  useGitopsKeys({
    sync: () => {
      if (writesDisabled) {
        return;
      }
      setChoosing(true);
    },
    refresh: () => {
      ask('refresh');
    },
    deepRefresh: () => {
      ask('hard-refresh');
    },
    terminate: () => {
      ask('terminate');
    },
  });

  return (
    <div className="shrink-0 border-b border-edge px-3 py-2 text-xs">
      <div className="flex flex-wrap items-center gap-2">
        <button
          type="button"
          onClick={() => {
            setChoosing(true);
          }}
          disabled={writesDisabled}
          title={blocked ?? undefined}
          className={buttonClass}
        >
          Sync
        </button>
        <button
          type="button"
          onClick={() => {
            ask('refresh');
          }}
          disabled={disabled}
          title={refused ?? undefined}
          className={buttonClass}
        >
          Refresh
        </button>
        <button
          type="button"
          onClick={() => {
            ask('hard-refresh');
          }}
          disabled={disabled}
          title={refused ?? 'Re-read the repository, ignoring the cache'}
          className={buttonClass}
        >
          Hard refresh
        </button>
        <button
          type="button"
          onClick={() => {
            ask('terminate');
          }}
          disabled={disabled}
          title={refused ?? 'Stop the running operation'}
          className={buttonClass}
        >
          Terminate
        </button>
        {suspended === true && (
          <button
            type="button"
            onClick={() => {
              ask('resume');
            }}
            disabled={writesDisabled}
            title={blocked ?? undefined}
            className={okClass}
          >
            Resume auto-sync
          </button>
        )}
        {suspended !== true && (
          <button
            type="button"
            onClick={() => {
              ask('suspend');
            }}
            disabled={writesDisabled}
            title={blocked ?? undefined}
            className={warnClass}
          >
            Suspend auto-sync
          </button>
        )}
        {suspended === true && <span className="text-warn-muted">auto-sync off</span>}
        {busy !== null && <span className="text-fg-muted">working</span>}
      </div>
      {choosing && (
        <ArgoSyncDialog
          name={target.name}
          onRun={(options) => {
            setChoosing(false);
            ask('sync', options);
          }}
          onCancel={() => {
            setChoosing(false);
          }}
        />
      )}
      {asking !== null && (
        <ConfirmByName
          open
          name={target.name}
          what={what(asking, target.name)}
          onConfirm={() => void run(asking, pending)}
          onCancel={() => {
            setAsking(null);
          }}
        />
      )}
      <Announce message={error} urgent className="mt-1.5 break-words text-error" />
      <Announce message={notice} className="mt-1.5 break-words text-ok" />
    </div>
  );
}

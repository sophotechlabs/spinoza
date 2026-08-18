import { useEffect, useRef, useState } from 'react';
import type { ObjectRef } from '../lib/types';
import type { ArgoAction } from '../lib/argoActions';
import { runArgoAction } from '../lib/argoActions';
import { confirmName } from '../lib/contexts';
import { useProtectedCluster } from '../store/contexts';
import Announce from './Announce';
import ConfirmByName from './ConfirmByName';

interface ArgoActionsProps {
  target: ObjectRef;
  onDone: () => void;
}

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'action failed';
}

function noticeFor(action: ArgoAction): string {
  if (action === 'sync') {
    return 'Sync requested.';
  }
  return 'Refresh requested.';
}

export default function ArgoActions({ target, onDone }: ArgoActionsProps) {
  const [busy, setBusy] = useState<ArgoAction | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [asking, setAsking] = useState(false);
  const protectedCluster = useProtectedCluster();
  const runRef = useRef(0);

  useEffect(() => {
    setError(null);
    setNotice(null);
    setAsking(false);
    runRef.current += 1;
  }, [target]);

  useEffect(() => {
    return () => {
      runRef.current += 1;
    };
  }, []);

  async function run(action: ArgoAction) {
    setBusy(action);
    setError(null);
    setNotice(null);
    setAsking(false);
    runRef.current += 1;
    const token = runRef.current;
    try {
      await runArgoAction(target, action, confirmName(protectedCluster, target.name));
      if (runRef.current !== token) {
        return;
      }
      setNotice(noticeFor(action));
      onDone();
    } catch (err: unknown) {
      if (runRef.current !== token) {
        return;
      }
      setError(errorMessage(err));
    } finally {
      setBusy(null);
    }
  }

  function askSync() {
    if (protectedCluster) {
      setAsking(true);
      return;
    }
    void run('sync');
  }

  const disabled = busy !== null;

  return (
    <div className="shrink-0 border-b border-edge px-3 py-2 text-xs">
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={askSync}
          disabled={disabled}
          className="rounded border border-edge-strong px-2 py-1 text-fg hover:bg-surface-active disabled:cursor-not-allowed disabled:text-fg-faint"
        >
          Sync
        </button>
        <button
          type="button"
          onClick={() => void run('refresh')}
          disabled={disabled}
          className="rounded border border-edge-strong px-2 py-1 text-fg hover:bg-surface-active disabled:cursor-not-allowed disabled:text-fg-faint"
        >
          Refresh
        </button>
        {busy !== null && <span className="text-fg-muted">working</span>}
      </div>
      {asking && (
        <ConfirmByName
          open
          name={target.name}
          what={`Syncing Application ${target.name} against its repository.`}
          onConfirm={() => void run('sync')}
          onCancel={() => {
            setAsking(false);
          }}
        />
      )}
      <Announce message={error} urgent className="mt-1.5 break-words text-error" />
      <Announce message={notice} className="mt-1.5 break-words text-ok" />
    </div>
  );
}

import { useEffect, useRef, useState } from 'react';
import { setProtection } from '../lib/contexts';
import { useContextList, useContextScope, useContextsStore } from '../store/contexts';
import { notifyError, notifyOk } from '../store/toasts';
import ClusterBadge from './ClusterBadge';

function reason(err: unknown): string {
  if (err instanceof Error && err.message !== '') {
    return err.message;
  }
  return 'the answer could not be saved';
}

export default function ProtectionPrompt() {
  const list = useContextList();
  const clusterScope = useContextScope();
  const setList = useContextsStore((state) => state.setList);
  const ref = useRef<HTMLDialogElement | null>(null);
  const operation = useRef(0);
  const liveScope = useRef(clusterScope);
  liveScope.current = clusterScope;
  const [busyScope, setBusyScope] = useState('');
  const busy = busyScope === clusterScope;
  const asking = list.protection === 'unknown' && list.current.name !== '';

  useEffect(() => {
    const scope = clusterScope;
    return () => {
      if (liveScope.current === scope) {
        liveScope.current = '';
      }
    };
  }, [clusterScope]);

  async function answer(protectedCluster: boolean) {
    const scope = clusterScope;
    const name = list.current.name;
    operation.current += 1;
    const token = operation.current;
    setBusyScope(scope);
    try {
      const found = await setProtection(protectedCluster);
      if (liveScope.current !== scope || operation.current !== token) {
        return;
      }
      setList(found);
      if (protectedCluster) {
        notifyOk(`${name} is protected`);
      }
    } catch (err: unknown) {
      if (liveScope.current !== scope || operation.current !== token) {
        return;
      }
      notifyError(reason(err));
    } finally {
      if (liveScope.current === scope && operation.current === token) {
        setBusyScope('');
      }
    }
  }

  useEffect(() => {
    const dialog = ref.current;
    if (dialog?.open === false) {
      dialog.showModal();
    }
  }, [asking]);

  if (!asking) {
    return null;
  }

  return (
    <dialog
      ref={ref}
      aria-label="Protect this cluster"
      className="backdrop:bg-black/50 m-auto w-[30rem] rounded border border-edge-strong bg-surface p-0 text-fg"
    >
      <div className="flex items-center gap-2 border-b border-edge px-3 py-2 text-xs font-semibold tracking-wide text-fg-strong uppercase">
        A cluster spinoza has not seen
        <ClusterBadge />
      </div>
      <div className="p-3 text-xs">
        <p className="text-fg-soft">
          <span className="font-semibold text-fg-strong">{list.current.name}</span> is new here. On
          a protected cluster, deleting, draining, scaling to zero and uninstalling need the object
          name typed first.
        </p>
        <div className="mt-3 flex items-center justify-end gap-2">
          <button
            type="button"
            disabled={busy}
            onClick={() => void answer(false)}
            className="rounded border border-edge-strong px-2 py-1 text-fg-soft hover:bg-surface-active disabled:text-fg-subtle"
          >
            Leave it open
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={() => void answer(true)}
            className="rounded border border-warn-line-strong px-2 py-1 text-warn-strong hover:bg-warn-tint disabled:text-fg-subtle"
          >
            Protect it
          </button>
        </div>
      </div>
    </dialog>
  );
}

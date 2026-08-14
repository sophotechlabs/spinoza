import { useEffect, useRef, useState } from 'react';
import { setProtection } from '../lib/contexts';
import { useContextList, useContextsStore } from '../store/contexts';
import { notifyError, notifyOk } from '../store/toasts';

function reason(err: unknown): string {
  if (err instanceof Error && err.message !== '') {
    return err.message;
  }
  return 'the answer could not be saved';
}

export default function ProtectionPrompt() {
  const list = useContextList();
  const setList = useContextsStore((state) => state.setList);
  const ref = useRef<HTMLDialogElement | null>(null);
  const [busy, setBusy] = useState(false);
  const asking = list.protection === 'unknown' && list.current.name !== '';

  useEffect(() => {
    const dialog = ref.current;
    if (dialog?.open === false) {
      dialog.showModal();
    }
  }, [asking]);

  async function answer(protectedCluster: boolean) {
    setBusy(true);
    try {
      setList(await setProtection(protectedCluster));
      if (protectedCluster) {
        notifyOk(`${list.current.name} is protected`);
      }
    } catch (err: unknown) {
      notifyError(reason(err));
    } finally {
      setBusy(false);
    }
  }

  if (!asking) {
    return null;
  }

  return (
    <dialog
      ref={ref}
      aria-label="Protect this cluster"
      className="backdrop:bg-black/50 m-auto w-[30rem] rounded border border-edge-strong bg-surface p-0 text-fg"
    >
      <div className="border-b border-edge px-3 py-2 text-xs font-semibold tracking-wide text-fg-strong uppercase">
        A cluster spinoza has not seen before
      </div>
      <div className="p-3 text-xs">
        <p className="text-fg-soft">
          <span className="font-semibold text-fg-strong">{list.current.name}</span> is new here. On
          a protected cluster, deleting an object, draining a node, scaling to zero and uninstalling
          a release all need its name typed first.
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

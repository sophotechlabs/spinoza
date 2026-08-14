import { useState } from 'react';
import { setProtection } from '../lib/contexts';
import { useContextList, useContextsStore } from '../store/contexts';
import { notifyError, notifyOk } from '../store/toasts';

function reason(err: unknown): string {
  if (err instanceof Error && err.message !== '') {
    return err.message;
  }
  return 'the answer could not be saved';
}

export default function ProtectedBadge() {
  const list = useContextList();
  const setList = useContextsStore((state) => state.setList);
  const [busy, setBusy] = useState(false);

  if (list.protection !== 'protected') {
    return null;
  }

  async function open() {
    setBusy(true);
    try {
      setList(await setProtection(false));
      notifyOk('This cluster is no longer protected');
    } catch (err: unknown) {
      notifyError(reason(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <button
      type="button"
      disabled={busy}
      onClick={() => void open()}
      title="Destructive actions on this cluster need its name typed. Click to lift that."
      className="rounded border border-warn-line-strong bg-warn-tint px-1.5 py-0.5 font-semibold tracking-wide text-warn-strong uppercase disabled:text-fg-subtle"
    >
      protected
    </button>
  );
}

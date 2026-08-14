import { useState } from 'react';
import { CONTROL } from '../lib/controls';
import { setProtection } from '../lib/contexts';
import { useContextList, useContextsStore } from '../store/contexts';
import { notifyError, notifyOk } from '../store/toasts';

const PROTECTED_CLASS = `${CONTROL} border-warn-line-strong bg-warn-tint font-semibold tracking-wide text-warn-strong uppercase disabled:text-fg-subtle`;

const OPEN_CLASS = `${CONTROL} border-edge-strong tracking-wide text-fg-muted uppercase hover:bg-surface-active disabled:text-fg-subtle`;

function reason(err: unknown): string {
  if (err instanceof Error && err.message !== '') {
    return err.message;
  }
  return 'the answer could not be saved';
}

function classFor(protectedCluster: boolean): string {
  if (protectedCluster) {
    return PROTECTED_CLASS;
  }
  return OPEN_CLASS;
}

function hintFor(protectedCluster: boolean): string {
  if (protectedCluster) {
    return 'Destructive actions on this cluster need its name typed. Click to lift that.';
  }
  return 'Destructive actions on this cluster run on one click. Click to ask for its name first.';
}

function labelFor(protectedCluster: boolean): string {
  if (protectedCluster) {
    return 'protected';
  }
  return 'open';
}

export default function ProtectionToggle() {
  const list = useContextList();
  const setList = useContextsStore((state) => state.setList);
  const [busy, setBusy] = useState(false);
  const protectedCluster = list.protection === 'protected';

  async function flip() {
    const wanted = !protectedCluster;
    setBusy(true);
    try {
      setList(await setProtection(wanted));
      if (wanted) {
        notifyOk(`${list.current.name} is protected`);
      } else {
        notifyOk(`${list.current.name} is open again`);
      }
    } catch (err: unknown) {
      notifyError(reason(err));
    } finally {
      setBusy(false);
    }
  }

  if (list.protection === 'unknown') {
    return null;
  }
  if (list.current.name === '') {
    return null;
  }

  return (
    <button
      type="button"
      disabled={busy}
      aria-pressed={protectedCluster}
      onClick={() => void flip()}
      title={hintFor(protectedCluster)}
      className={classFor(protectedCluster)}
    >
      {labelFor(protectedCluster)}
    </button>
  );
}

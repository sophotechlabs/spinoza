import { useState } from 'react';
import { ICON_CONTROL } from '../lib/controls';
import { setProtection } from '../lib/contexts';
import { useContextList, useContextsStore } from '../store/contexts';
import { notifyError, notifyOk } from '../store/toasts';
import { LockedIcon, UnlockedIcon } from './icons';

const PROTECTED_CLASS = `${ICON_CONTROL} border-ok-line bg-ok-tint text-ok hover:bg-ok-emphasis disabled:text-fg-subtle`;

const OPEN_CLASS = `${ICON_CONTROL} border-warn-line text-warn-muted hover:bg-warn-tint disabled:text-fg-subtle`;

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
    return 'Destructive actions need the name typed. Click to lift.';
  }
  return 'Destructive actions run on one click. Click to protect.';
}

function labelFor(protectedCluster: boolean): string {
  if (protectedCluster) {
    return 'Protected cluster';
  }
  return 'Open cluster';
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
      aria-label={labelFor(protectedCluster)}
      onClick={() => void flip()}
      title={hintFor(protectedCluster)}
      className={classFor(protectedCluster)}
    >
      {protectedCluster && <LockedIcon />}
      {!protectedCluster && <UnlockedIcon />}
    </button>
  );
}

import { useEffect, useRef, useState } from 'react';
import { ICON_CONTROL } from '../lib/controls';
import { setProtection } from '../lib/contexts';
import { useContextList, useContextScope, useContextsStore } from '../store/contexts';
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
  const clusterScope = useContextScope();
  const setList = useContextsStore((state) => state.setList);
  const protectedCluster = list.protection === 'protected';
  const operation = useRef(0);
  const liveScope = useRef(clusterScope);
  liveScope.current = clusterScope;
  const [busyScope, setBusyScope] = useState('');
  const busy = busyScope === clusterScope;

  useEffect(() => {
    const scope = clusterScope;
    return () => {
      if (liveScope.current === scope) {
        liveScope.current = '';
      }
    };
  }, [clusterScope]);

  async function flip() {
    const wanted = !protectedCluster;
    const scope = clusterScope;
    const name = list.current.name;
    operation.current += 1;
    const token = operation.current;
    setBusyScope(scope);
    try {
      const found = await setProtection(wanted);
      if (liveScope.current !== scope || operation.current !== token) {
        return;
      }
      setList(found);
      if (wanted) {
        notifyOk(`${name} is protected`);
      } else {
        notifyOk(`${name} is open again`);
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

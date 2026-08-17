import { useEffect, useRef, useState } from 'react';
import type { DebugSession, ExecTarget } from '../lib/types';
import {
  DEBUG_PROFILES,
  DEFAULT_PROFILE,
  fetchDebugSupport,
  startDebug,
} from '../lib/debugContainer';
import type { DebugProfile } from '../lib/debugContainer';
import { notifyWarn } from '../store/toasts';

interface DebugPromptProps {
  target: ExecTarget;
  onAttached: (container: string) => void;
}

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'starting a debug container failed';
}

function supportMessage(err: unknown): string {
  if (err instanceof Error) {
    return `Could not check whether debug containers are allowed here: ${err.message}`;
  }
  return 'Could not check whether debug containers are allowed here.';
}

function reuseNotice(session: DebugSession, wanted: DebugProfile): string | null {
  if (session.created) {
    return null;
  }
  if (session.profile === '' || session.profile === wanted) {
    return null;
  }
  return `Attached to the existing ${session.container}, which runs under the ${session.profile} profile, not ${wanted}. An ephemeral container cannot be changed once it exists.`;
}

function refusalMessage(namespace: string, reason?: string): string {
  const base = `Your kubeconfig cannot add debug containers in ${namespace}.`;
  if (reason === undefined || reason === '') {
    return base;
  }
  return `${base} ${reason}`;
}

function buttonLabel(busy: boolean): string {
  if (busy) {
    return 'Starting';
  }
  return 'Attach debug container';
}

export default function DebugPrompt({ target, onAttached }: DebugPromptProps) {
  const [profile, setProfile] = useState<DebugProfile>(DEFAULT_PROFILE);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [refused, setRefused] = useState<string | null>(null);
  const [unchecked, setUnchecked] = useState<string | null>(null);
  const [image, setImage] = useState('');

  const { namespace, pod } = target;
  const targetKey = `${namespace}/${pod}/${target.container}`;
  const liveTargetRef = useRef(targetKey);

  useEffect(() => {
    liveTargetRef.current = targetKey;
    return () => {
      liveTargetRef.current = '';
    };
  }, [targetKey]);

  useEffect(() => {
    let live = true;
    fetchDebugSupport(namespace, pod)
      .then((support) => {
        if (!live) {
          return;
        }
        setUnchecked(null);
        setImage(support.image);
        if (support.allowed) {
          setRefused(null);
          return;
        }
        setRefused(refusalMessage(namespace, support.reason));
      })
      .catch((err: unknown) => {
        if (!live) {
          return;
        }
        setUnchecked(supportMessage(err));
      });
    return () => {
      live = false;
    };
  }, [namespace, pod]);

  async function attach() {
    const key = targetKey;
    setBusy(true);
    setError(null);
    try {
      const session = await startDebug(target, profile);
      if (liveTargetRef.current !== key) {
        return;
      }
      const reused = reuseNotice(session, profile);
      if (reused !== null) {
        notifyWarn(reused);
      }
      onAttached(session.container);
    } catch (err: unknown) {
      if (liveTargetRef.current !== key) {
        return;
      }
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  function handleProfile(event: React.ChangeEvent<HTMLSelectElement>) {
    setProfile(event.target.value as DebugProfile);
  }

  return (
    <div className="min-h-0 flex-1 overflow-auto border-t border-edge bg-surface p-3 text-[11px]">
      <p className="text-fg-soft">
        {target.container} has no shell, so it cannot be exec&apos;d into.
      </p>
      <p className="mt-1 text-fg-muted">
        Kubernetes can add a temporary container beside it, sharing its processes, network and
        filesystem. It stays on the pod until the pod is replaced.
      </p>
      <div className="mt-2.5 flex items-center gap-2">
        <button
          type="button"
          onClick={() => void attach()}
          disabled={busy || refused !== null}
          className="rounded border border-edge-strong px-2 py-1 text-fg hover:bg-surface-active disabled:cursor-not-allowed disabled:text-fg-faint"
        >
          {buttonLabel(busy)}
        </button>
        <label className="text-fg-muted" htmlFor="debug-profile">
          profile
        </label>
        <select
          id="debug-profile"
          aria-label="Debug profile"
          value={profile}
          onChange={handleProfile}
          className="rounded border border-edge-strong bg-surface-raised px-1 py-0.5 text-fg"
        >
          {DEBUG_PROFILES.map((name) => (
            <option key={name} value={name}>
              {name}
            </option>
          ))}
        </select>
        <span className="truncate text-fg-muted">{image}</span>
      </div>
      {profile === 'sysadmin' && (
        <p className="mt-1.5 text-warn">
          sysadmin runs the debug container privileged. general already grants the target&apos;s
          processes, network and filesystem.
        </p>
      )}
      {unchecked !== null && (
        <p role="status" className="mt-1.5 break-words text-warn">
          {unchecked} Attaching may still work; the failure will say why if it does not.
        </p>
      )}
      {refused !== null && <p className="mt-1.5 break-words text-error">{refused}</p>}
      {error !== null && <p className="mt-1.5 break-words text-error">{error}</p>}
    </div>
  );
}

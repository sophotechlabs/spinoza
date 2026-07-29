import { useEffect, useState } from 'react';
import type { ExecTarget } from '../lib/types';
import {
  DEBUG_PROFILES,
  DEFAULT_PROFILE,
  fetchDebugSupport,
  startDebug,
} from '../lib/debugContainer';
import type { DebugProfile } from '../lib/debugContainer';

interface DebugPromptProps {
  target: ExecTarget;
  image: string;
  onAttached: (container: string) => void;
}

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'starting a debug container failed';
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
    return 'Starting…';
  }
  return 'Attach debug container';
}

export default function DebugPrompt({ target, image, onAttached }: DebugPromptProps) {
  const [profile, setProfile] = useState<DebugProfile>(DEFAULT_PROFILE);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [refused, setRefused] = useState<string | null>(null);

  const { namespace } = target;

  useEffect(() => {
    let live = true;
    fetchDebugSupport(namespace)
      .then((support) => {
        if (!live) {
          return;
        }
        if (support.allowed) {
          setRefused(null);
          return;
        }
        setRefused(refusalMessage(namespace, support.reason));
      })
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [namespace]);

  async function attach() {
    setBusy(true);
    setError(null);
    try {
      const session = await startDebug(target, profile);
      onAttached(session.container);
    } catch (err: unknown) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  function handleProfile(event: React.ChangeEvent<HTMLSelectElement>) {
    setProfile(event.target.value as DebugProfile);
  }

  return (
    <div className="h-56 overflow-auto border-t border-neutral-800 bg-neutral-950 p-3 text-[11px]">
      <p className="text-neutral-300">
        {target.container} has no shell, so it cannot be exec&apos;d into.
      </p>
      <p className="mt-1 text-neutral-500">
        Kubernetes can add a temporary container beside it, sharing its processes, network and
        filesystem. It cannot be removed afterwards — it stays on the pod until the pod is replaced.
      </p>
      <div className="mt-2.5 flex items-center gap-2">
        <button
          type="button"
          onClick={() => void attach()}
          disabled={busy || refused !== null}
          className="rounded border border-neutral-700 px-2 py-1 text-neutral-200 hover:bg-neutral-800 disabled:cursor-not-allowed disabled:text-neutral-600"
        >
          {buttonLabel(busy)}
        </button>
        <label className="text-neutral-500" htmlFor="debug-profile">
          profile
        </label>
        <select
          id="debug-profile"
          aria-label="Debug profile"
          value={profile}
          onChange={handleProfile}
          className="rounded border border-neutral-700 bg-neutral-900 px-1 py-0.5 text-neutral-200"
        >
          {DEBUG_PROFILES.map((name) => (
            <option key={name} value={name}>
              {name}
            </option>
          ))}
        </select>
        <span className="truncate text-neutral-600">{image}</span>
      </div>
      {profile === 'sysadmin' && (
        <p className="mt-1.5 text-amber-400">
          sysadmin runs the debug container privileged. general already grants the target&apos;s
          processes, network and filesystem.
        </p>
      )}
      {refused !== null && <p className="mt-1.5 break-words text-red-400">{refused}</p>}
      {error !== null && <p className="mt-1.5 break-words text-red-400">{error}</p>}
    </div>
  );
}

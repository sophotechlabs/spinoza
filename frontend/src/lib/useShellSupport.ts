import { useCallback, useEffect, useState } from 'react';
import type { ShellState } from './types';
import { fetchExecSupport } from './exec';
import { useClusterEpoch } from '../store/cluster';

export interface ShellSupport {
  shell: ShellState;
  error: string | null;
  markMissing: () => void;
}

function probeMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'the shell probe failed';
}

export function useShellSupport(namespace: string, pod: string, container: string): ShellSupport {
  const epoch = useClusterEpoch();
  const [shell, setShell] = useState<ShellState>('unknown');
  const [error, setError] = useState<string | null>(null);
  const [askedFor, setAskedFor] = useState('');
  const targetKey = `${epoch}|${namespace}|${pod}|${container}`;

  useEffect(() => {
    setShell('unknown');
    setError(null);
    if (pod === '') {
      return;
    }
    if (container === '') {
      return;
    }
    let live = true;
    fetchExecSupport({ namespace, pod, container })
      .then((support) => {
        if (live) {
          setShell(support.shell);
          setAskedFor(targetKey);
        }
      })
      .catch((err: unknown) => {
        if (live) {
          setError(probeMessage(err));
          setAskedFor(targetKey);
        }
      });
    return () => {
      live = false;
    };
  }, [namespace, pod, container, targetKey, epoch]);

  const markMissing = useCallback(() => {
    setShell('absent');
    setAskedFor(targetKey);
  }, [targetKey]);

  if (pod === '' || container === '' || askedFor !== targetKey) {
    return { shell: 'unknown', error: null, markMissing };
  }
  return { shell, error, markMissing };
}

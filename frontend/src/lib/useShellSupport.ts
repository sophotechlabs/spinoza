import { useCallback, useEffect, useState } from 'react';
import type { ShellState } from './types';
import { fetchExecSupport } from './exec';

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
  const [shell, setShell] = useState<ShellState>('unknown');
  const [error, setError] = useState<string | null>(null);

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
        }
      })
      .catch((err: unknown) => {
        if (live) {
          setError(probeMessage(err));
        }
      });
    return () => {
      live = false;
    };
  }, [namespace, pod, container]);

  const markMissing = useCallback(() => {
    setShell('absent');
  }, []);

  return { shell, error, markMissing };
}

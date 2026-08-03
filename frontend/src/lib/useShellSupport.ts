import { useCallback, useEffect, useState } from 'react';
import type { ShellState } from './types';
import { fetchExecSupport } from './exec';

export interface ShellSupport {
  shell: ShellState;
  markMissing: () => void;
}

export function useShellSupport(namespace: string, pod: string, container: string): ShellSupport {
  const [shell, setShell] = useState<ShellState>('unknown');

  useEffect(() => {
    setShell('unknown');
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
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [namespace, pod, container]);

  const markMissing = useCallback(() => {
    setShell('absent');
  }, []);

  return { shell, markMissing };
}

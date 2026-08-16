import { useEffect, useState } from 'react';
import type { LocalShell } from './types';
import { fetchLocalShellSupport } from './exec';

const DESKTOP_ONLY = 'A shell on this machine is only available in the desktop app.';

export function useLocalShellSupport(): LocalShell | null {
  const [support, setSupport] = useState<LocalShell | null>(null);

  useEffect(() => {
    let live = true;
    fetchLocalShellSupport()
      .then((found) => {
        if (live) {
          setSupport(found);
        }
      })
      .catch(() => {
        if (live) {
          setSupport({ available: false, reason: DESKTOP_ONLY });
        }
      });
    return () => {
      live = false;
    };
  }, []);

  return support;
}

import { useEffect, useRef } from 'react';
import { ignorable } from './hotkeys';

export interface GitopsKeys {
  sync?: () => void;
  refresh?: () => void;
  deepRefresh?: () => void;
  terminate?: () => void;
}

function chosen(event: KeyboardEvent, keys: GitopsKeys): (() => void) | undefined {
  if (event.key === 'S' || event.key === 's') {
    return keys.sync;
  }
  if (event.key === 'R') {
    return keys.deepRefresh;
  }
  if (event.key === 'r') {
    return keys.refresh;
  }
  if (event.key === 'T' || event.key === 't') {
    return keys.terminate;
  }
  return undefined;
}

export function useGitopsKeys(keys: GitopsKeys): void {
  const latest = useRef(keys);
  latest.current = keys;

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (ignorable(event)) {
        return;
      }
      const act = chosen(event, latest.current);
      if (act === undefined) {
        return;
      }
      event.preventDefault();
      act();
    }
    window.addEventListener('keydown', onKeyDown);
    return () => {
      window.removeEventListener('keydown', onKeyDown);
    };
  }, []);
}

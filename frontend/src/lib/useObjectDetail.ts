import { useEffect, useState } from 'react';
import type { ObjectDetail, ObjectRef } from './types';
import { fetchObject, refQuery } from './object';

export interface ObjectDetailState {
  detail: ObjectDetail | null;
  error: string | null;
  reload: () => void;
}

function keyOf(target: ObjectRef | null): string {
  if (target === null) {
    return '';
  }
  return refQuery(target);
}

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'object request failed';
}

export function useObjectDetail(target: ObjectRef | null): ObjectDetailState {
  const [detail, setDetail] = useState<ObjectDetail | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [reloads, setReloads] = useState(0);

  const targetKey = keyOf(target);
  const [lastKey, setLastKey] = useState(targetKey);
  if (targetKey !== lastKey) {
    setLastKey(targetKey);
    setDetail(null);
    setError(null);
  }

  useEffect(() => {
    if (target === null) {
      setDetail(null);
      setError(null);
      return;
    }
    let mounted = true;
    const load = async () => {
      try {
        const next = await fetchObject(target);
        if (mounted) {
          setDetail(next);
          setError(null);
        }
      } catch (err: unknown) {
        if (mounted) {
          setDetail(null);
          setError(errorMessage(err));
        }
      }
    };
    void load();
    return () => {
      mounted = false;
    };
  }, [target, reloads]);

  function reload() {
    setReloads((value) => value + 1);
  }

  return { detail, error, reload };
}

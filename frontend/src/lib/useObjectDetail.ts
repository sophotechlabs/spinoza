import { useEffect, useState } from 'react';
import type { ObjectDetail, ObjectRef, Row } from './types';
import { fetchObject, refQuery } from './object';
import { useClusterEpoch } from '../store/cluster';

export interface ObjectDetailState {
  detail: ObjectDetail | null;
  error: string | null;
  gone: boolean;
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

export function useObjectDetail(target: ObjectRef | null, live: Row | null): ObjectDetailState {
  const epoch = useClusterEpoch();
  const [detail, setDetail] = useState<ObjectDetail | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [gone, setGone] = useState(false);
  const [reloads, setReloads] = useState(0);
  const [watched, setWatched] = useState(live);

  const targetKey = `${epoch}|${keyOf(target)}`;
  const [lastKey, setLastKey] = useState(targetKey);
  if (targetKey !== lastKey) {
    setLastKey(targetKey);
    setDetail(null);
    setError(null);
    setGone(false);
    setWatched(live);
  } else if (live !== watched) {
    setWatched(live);
    if (live === null) {
      setDetail(null);
      setError(null);
      setGone(true);
    } else {
      setGone(false);
      setReloads((value) => value + 1);
    }
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
  }, [target, reloads, epoch]);

  function reload() {
    setReloads((value) => value + 1);
  }

  return { detail, error, gone, reload };
}

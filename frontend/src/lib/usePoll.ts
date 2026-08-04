import { useCallback, useEffect, useState } from 'react';
import { useClusterEpoch } from '../store/cluster';

export interface Polled<T> {
  data: T | null;
  error: string | null;
  stale: boolean;
  reload: () => void;
}

export interface PollOptions {
  intervalMs: number;
  enabled?: boolean;
  fallback?: string;
}

function messageOf(err: unknown, fallback: string): string {
  if (err instanceof Error) {
    return err.message;
  }
  return fallback;
}

export function usePoll<T>(fetcher: () => Promise<T>, options: PollOptions): Polled<T> {
  const { intervalMs, enabled = true, fallback = 'request failed' } = options;
  const epoch = useClusterEpoch();
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [reloads, setReloads] = useState(0);
  const [lastEpoch, setLastEpoch] = useState(epoch);

  if (epoch !== lastEpoch) {
    setLastEpoch(epoch);
    setData(null);
    setError(null);
  }

  useEffect(() => {
    if (!enabled) {
      setData(null);
      setError(null);
      return;
    }
    let mounted = true;
    let inFlight = false;
    const load = async () => {
      if (inFlight) {
        return;
      }
      inFlight = true;
      try {
        const next = await fetcher();
        if (mounted) {
          setData(next);
          setError(null);
        }
      } catch (err: unknown) {
        if (mounted) {
          setError(messageOf(err, fallback));
        }
      } finally {
        inFlight = false;
      }
    };
    void load();
    const timer = setInterval(() => {
      void load();
    }, intervalMs);
    return () => {
      mounted = false;
      clearInterval(timer);
    };
  }, [fetcher, intervalMs, enabled, fallback, epoch, reloads]);

  const reload = useCallback(() => {
    setReloads((value) => value + 1);
  }, []);

  let stale = false;
  if (data !== null && error !== null) {
    stale = true;
  }

  return { data, error, stale, reload };
}

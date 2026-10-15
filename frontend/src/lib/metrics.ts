import { useCallback, useEffect, useState } from 'react';
import type { Metrics } from './types';
import { request } from './http';
import { parseMetrics } from './parse';
import { usePoll } from './usePoll';
import type { Polled } from './usePoll';
import { failure } from './object';

const METRICS_POLL_MS = 10000;
export const METRICS_CUTOFF_MS = 60000;

function metricsPath(namespace: string): string {
  if (namespace === '') {
    return '/api/metrics';
  }
  const params = new URLSearchParams({ namespace });
  return `/api/metrics?${params.toString()}`;
}

export async function fetchMetrics(namespace = ''): Promise<Metrics> {
  const response = await request(metricsPath(namespace));
  if (!response.ok) {
    throw await failure(response, `metrics request failed with status ${response.status}`);
  }
  return parseMetrics(await response.json());
}

export function isUsable(data: Metrics): boolean {
  if (data.error === undefined) {
    return true;
  }
  if (data.error === '') {
    return true;
  }
  if (Object.keys(data.pods).length > 0) {
    return true;
  }
  return Object.keys(data.nodes).length > 0;
}

export async function loadMetrics(namespace = ''): Promise<Metrics> {
  const data = await fetchMetrics(namespace);
  const problem = data.error;
  if (problem === undefined) {
    return data;
  }
  if (isUsable(data)) {
    return data;
  }
  throw new Error(problem);
}

function useExpiry(polled: Polled<Metrics>, cutoffMs: number): boolean {
  const [expired, setExpired] = useState(false);
  const { data, error } = polled;

  useEffect(() => {
    if (data === null) {
      setExpired(false);
      return;
    }
    if (error === null) {
      setExpired(false);
      return;
    }
    const timer = setTimeout(() => {
      setExpired(true);
    }, cutoffMs);
    return () => {
      clearTimeout(timer);
    };
  }, [data, error, cutoffMs]);

  return expired;
}

export function useMetrics(enabled: boolean, namespace = ''): Polled<Metrics> {
  const load = useCallback(() => loadMetrics(namespace), [namespace]);
  const polled = usePoll(load, {
    intervalMs: METRICS_POLL_MS,
    enabled,
    fallback: 'metrics request failed',
    resetKey: namespace,
  });
  const expired = useExpiry(polled, METRICS_CUTOFF_MS);

  if (!expired) {
    return polled;
  }
  return {
    data: null,
    askedFor: polled.askedFor,
    error: polled.error,
    stale: true,
    reload: polled.reload,
  };
}

export function barColor(percent: number): string {
  if (percent >= 90) {
    return 'bg-error-solid';
  }
  if (percent >= 70) {
    return 'bg-warn-solid';
  }
  return 'bg-ok-solid';
}

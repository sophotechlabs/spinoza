import { useEffect, useState } from 'react';
import type { Metrics } from './types';

const METRICS_POLL_MS = 10000;

export async function fetchMetrics(): Promise<Metrics> {
  const response = await fetch('/api/metrics');
  if (!response.ok) {
    throw new Error(`metrics request failed with status ${response.status}`);
  }
  const data = (await response.json()) as Metrics;
  return data;
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

export function useMetrics(enabled: boolean): Metrics | null {
  const [metrics, setMetrics] = useState<Metrics | null>(null);
  useEffect(() => {
    if (!enabled) {
      setMetrics(null);
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
        const data = await fetchMetrics();
        if (!mounted) {
          return;
        }
        if (!isUsable(data)) {
          return;
        }
        setMetrics(data);
      } catch {
        return;
      } finally {
        inFlight = false;
      }
    };
    void load();
    const timer = setInterval(() => {
      void load();
    }, METRICS_POLL_MS);
    return () => {
      mounted = false;
      clearInterval(timer);
    };
  }, [enabled]);
  return metrics;
}

export function formatCpu(milli: number): string {
  if (milli <= 0) {
    return '';
  }
  return `${milli}m`;
}

export function formatMem(mi: number): string {
  if (mi <= 0) {
    return '';
  }
  if (mi >= 1024) {
    return `${(mi / 1024).toFixed(1)}Gi`;
  }
  return `${mi}Mi`;
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

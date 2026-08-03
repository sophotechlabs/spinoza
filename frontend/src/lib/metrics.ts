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

export function useMetrics(enabled: boolean): Metrics | null {
  const [metrics, setMetrics] = useState<Metrics | null>(null);
  useEffect(() => {
    if (!enabled) {
      setMetrics(null);
      return;
    }
    let mounted = true;
    const load = async () => {
      try {
        const data = await fetchMetrics();
        if (mounted) {
          setMetrics(data);
        }
      } catch {
        return;
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
    return 'bg-red-500';
  }
  if (percent >= 70) {
    return 'bg-yellow-500';
  }
  return 'bg-green-500';
}

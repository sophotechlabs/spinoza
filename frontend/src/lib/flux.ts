import { useEffect, useState } from 'react';
import type { FluxDashboard } from './types';

const FLUX_POLL_MS = 5000;

export async function fetchFlux(): Promise<FluxDashboard> {
  const response = await fetch('/api/flux');
  if (!response.ok) {
    throw new Error(`flux request failed with status ${response.status}`);
  }
  const data = (await response.json()) as FluxDashboard;
  return data;
}

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'flux request failed';
}

export function useFlux(): { data: FluxDashboard | null; error: string | null } {
  const [data, setData] = useState<FluxDashboard | null>(null);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    let mounted = true;
    let inFlight = false;
    const load = async () => {
      if (inFlight) {
        return;
      }
      inFlight = true;
      try {
        const dash = await fetchFlux();
        if (mounted) {
          setData(dash);
          setError(null);
        }
      } catch (err: unknown) {
        if (mounted) {
          setError(errorMessage(err));
        }
      } finally {
        inFlight = false;
      }
    };
    void load();
    const timer = setInterval(() => {
      void load();
    }, FLUX_POLL_MS);
    return () => {
      mounted = false;
      clearInterval(timer);
    };
  }, []);
  return { data, error };
}

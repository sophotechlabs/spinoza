import { useEffect } from 'react';
import { fetchTrafficSupport } from './traffic';
import { rememberTrafficSupport } from '../store/traffic';
import { useClusterEpoch } from '../store/cluster';

const PROBE_INTERVAL_MS = 30000;

const PROBE_FAILED = 'the traffic probe failed';

const CHECKING = 'checking whether a service mesh is exporting flow metrics';

function probeMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return PROBE_FAILED;
}

export function useTrafficProbe(): void {
  const epoch = useClusterEpoch();

  useEffect(() => {
    let mounted = true;
    rememberTrafficSupport({ available: false, reason: CHECKING });
    const load = async () => {
      try {
        const support = await fetchTrafficSupport();
        if (mounted) {
          rememberTrafficSupport(support);
        }
      } catch (err: unknown) {
        if (mounted) {
          rememberTrafficSupport({ available: false, reason: probeMessage(err) });
        }
      }
    };
    void load();
    const timer = setInterval(() => {
      void load();
    }, PROBE_INTERVAL_MS);
    return () => {
      mounted = false;
      clearInterval(timer);
    };
  }, [epoch]);
}

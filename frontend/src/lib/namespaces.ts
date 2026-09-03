import { useEffect } from 'react';
import type { Namespaces } from './types';
import { failure } from './object';
import { request } from './http';
import { useClusterEpoch } from '../store/cluster';
import { useActiveCluster } from '../store/clusters';
import { useNamespaceStore } from '../store/namespace';
import { notifyError } from '../store/toasts';

export async function fetchNamespaces(): Promise<Namespaces> {
  const response = await request('/api/namespaces');
  if (!response.ok) {
    throw await failure(response, `the namespace request failed with status ${response.status}`);
  }
  const body = (await response.json()) as Partial<Namespaces>;
  return { names: body.names ?? [], error: body.error };
}

export function useNamespaces(): void {
  const cluster = useActiveCluster();
  const epoch = useClusterEpoch();
  const offer = useNamespaceStore((state) => state.offer);

  useEffect(() => {
    if (cluster === '') {
      return;
    }
    let live = true;
    fetchNamespaces()
      .then((found) => {
        if (!live) {
          return;
        }
        if (found.error !== undefined && found.error !== '') {
          notifyError(`Listing namespaces: ${found.error}`);
          return;
        }
        offer(cluster, found.names);
      })
      .catch((err: unknown) => {
        if (live) {
          let message = 'the namespace request failed';
          if (err instanceof Error && err.message !== '') {
            message = err.message;
          }
          notifyError(`Listing namespaces: ${message}`);
        }
      });
    return () => {
      live = false;
    };
  }, [cluster, epoch, offer]);
}

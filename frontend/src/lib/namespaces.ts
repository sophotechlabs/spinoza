import { useEffect } from 'react';
import type { Namespaces } from './types';
import { failure } from './object';
import { request } from './http';
import { useClusterEpoch } from '../store/cluster';
import { useActiveCluster } from '../store/clusters';
import { useNamespaceStore } from '../store/namespace';

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
        if (live) {
          offer(cluster, found.names);
        }
      })
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [cluster, epoch, offer]);
}

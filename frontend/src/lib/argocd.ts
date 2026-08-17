import { useEffect, useState } from 'react';
import type { ArgoApp, ArgoDashboard, ObjectRef } from './types';
import { failure } from './object';
import { request } from './http';
import { useClusterEpoch } from '../store/cluster';

const REFRESH_MS = 10000;

export interface ArgoTree {
  app: ArgoApp;
  depth: number;
}

function appOf(raw: unknown): ArgoApp {
  const item = raw as Partial<ArgoApp>;
  return {
    kind: item.kind ?? '',
    group: item.group ?? '',
    version: item.version ?? '',
    resource: item.resource ?? '',
    name: item.name ?? '',
    namespace: item.namespace ?? '',
    project: item.project ?? '',
    sync: item.sync ?? '',
    health: item.health ?? '',
    revision: item.revision ?? '',
    repo: item.repo ?? '',
    path: item.path ?? '',
    destination: item.destination ?? '',
    message: item.message ?? '',
    owner: item.owner,
    createdAt: item.createdAt ?? '',
  };
}

export function refOf(app: ArgoApp): ObjectRef {
  return {
    group: app.group,
    version: app.version,
    resource: app.resource,
    namespace: app.namespace,
    name: app.name,
  };
}

export async function fetchArgo(): Promise<ArgoDashboard> {
  const response = await request('/api/argocd');
  if (!response.ok) {
    throw await failure(response, `the argo request failed with status ${response.status}`);
  }
  const body = (await response.json()) as Partial<ArgoDashboard>;
  return {
    apps: (body.apps ?? []).map(appOf),
    applicationSets: (body.applicationSets ?? []).map(appOf),
    error: body.error,
  };
}

export function tree(apps: ArgoApp[]): ArgoTree[] {
  const children = new Map<string, ArgoApp[]>();
  const roots: ArgoApp[] = [];
  for (const app of apps) {
    const owner = app.owner ?? '';
    if (owner === '') {
      roots.push(app);
      continue;
    }
    const kin = children.get(owner) ?? [];
    kin.push(app);
    children.set(owner, kin);
  }
  const out: ArgoTree[] = [];
  const seen = new Set<string>();

  function walk(app: ArgoApp, depth: number): void {
    if (seen.has(app.name)) {
      return;
    }
    seen.add(app.name);
    out.push({ app, depth });
    for (const child of children.get(app.name) ?? []) {
      walk(child, depth + 1);
    }
  }

  for (const root of roots) {
    walk(root, 0);
  }
  for (const app of apps) {
    walk(app, 0);
  }
  return out;
}

export function useArgo(): { data: ArgoDashboard | null; error: string | null } {
  const epoch = useClusterEpoch();
  const [data, setData] = useState<ArgoDashboard | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    function load() {
      fetchArgo()
        .then((found) => {
          if (live) {
            setData(found);
            setError(null);
          }
        })
        .catch((err: unknown) => {
          if (live) {
            setError(err instanceof Error ? err.message : 'the argo request failed');
          }
        });
    }
    load();
    const timer = setInterval(load, REFRESH_MS);
    return () => {
      live = false;
      clearInterval(timer);
    };
  }, [epoch]);

  return { data, error };
}

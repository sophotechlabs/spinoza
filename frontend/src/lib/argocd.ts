import type {
  ArgoApp,
  ArgoDashboard,
  Graph,
  GraphEdge,
  GraphNode,
  ObjectRef,
  ReadyState,
} from './types';
import { failure } from './object';
import { request } from './http';
import { usePoll } from './usePoll';

const REFRESH_MS = 10000;

export interface ArgoTree {
  app: ArgoApp;
  depth: number;
}

function appOf(raw: unknown): ArgoApp {
  const item = raw as Partial<ArgoApp>;
  return {
    kind: item.kind ?? '',
    automation: item.automation,
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
    projects: (body.projects ?? []).map(appOf),
    error: body.error,
  };
}

export function idOf(app: ArgoApp): string {
  return `${app.resource}/${app.namespace}/${app.name}`;
}

export function readyOf(app: ArgoApp): ReadyState {
  if (app.health === 'Degraded' || app.health === 'Missing') {
    return 'False';
  }
  if (app.health === 'Healthy' && app.sync === 'Synced') {
    return 'True';
  }
  return 'Unknown';
}

export function statusOf(app: ArgoApp): string {
  return [app.sync, app.health].filter((part) => part !== '').join(' ');
}

function nodeOf(app: ArgoApp): GraphNode {
  let category: GraphNode['category'] = 'app';
  if (app.kind === 'ApplicationSet') {
    category = 'applier';
  }
  return {
    id: idOf(app),
    kind: app.kind,
    group: app.group,
    version: app.version,
    resource: app.resource,
    name: app.name,
    namespace: app.namespace,
    status: statusOf(app),
    ready: readyOf(app),
    category,
    contains: 0,
    unhealthy: 0,
  };
}

export function graphOf(data: ArgoDashboard): Graph {
  const owners = new Map<string, ArgoApp>();
  for (const app of [...data.applicationSets, ...data.apps]) {
    owners.set(app.name, app);
  }
  const nodes = [...data.applicationSets, ...data.apps].map(nodeOf);
  const edges: GraphEdge[] = [];
  for (const app of data.apps) {
    const owner = owners.get(app.owner ?? '');
    if (owner === undefined) {
      continue;
    }
    edges.push({ from: idOf(owner), to: idOf(app), kind: 'manages' as const });
  }
  return { nodes, edges, error: data.error };
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

export interface ArgoState {
  data: ArgoDashboard | null;
  error: string | null;
  reload: () => void;
}

export function useArgo(): ArgoState {
  return usePoll(fetchArgo, {
    intervalMs: REFRESH_MS,
    fallback: 'the argo request failed',
  });
}

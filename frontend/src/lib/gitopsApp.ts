import { useCallback, useEffect, useState } from 'react';
import { SEVERITIES } from './types';
import type {
  FieldDrift,
  GitopsApp,
  GitopsDeployment,
  GitopsIssue,
  GitopsOperation,
  GitopsResource,
  Graph,
  ObjectRef,
} from './types';
import { failure, refQuery } from './object';
import { oneOf } from './wire';
import { parseEvents, parseGraph } from './parse';
import { request } from './http';
import { isArgoApplication } from './argoActions';
import { groupOf } from './fluxActions';
import { useClusterEpoch } from '../store/cluster';

const REFRESH_MS = 10000;

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'the application request failed';
}

const FLUX_APPLIERS: Record<string, string> = {
  'kustomize.toolkit.fluxcd.io': 'Kustomization',
  'helm.toolkit.fluxcd.io': 'HelmRelease',
};

export function isGitopsApp(apiVersion: string, kind: string): boolean {
  if (isArgoApplication(apiVersion, kind)) {
    return true;
  }
  return FLUX_APPLIERS[groupOf(apiVersion)] === kind;
}

function driftOf(raw: unknown): FieldDrift {
  const item = raw as Partial<FieldDrift>;
  return { path: item.path ?? '', declared: item.declared ?? '', live: item.live ?? '' };
}

function resourceOf(raw: unknown): GitopsResource {
  const item = raw as Partial<GitopsResource>;
  return {
    group: item.group,
    version: item.version,
    resource: item.resource,
    kind: item.kind ?? '',
    name: item.name ?? '',
    namespace: item.namespace,
    sync: item.sync,
    health: item.health,
    message: item.message,
    terminating: item.terminating,
    finalizers: item.finalizers,
    drift: (item.drift ?? []).map(driftOf),
    driftOwners: item.driftOwners,
    driftNote: item.driftNote,
    events: parseEvents(item.events ?? []),
  };
}

function issueOf(raw: unknown): GitopsIssue {
  const item = raw as Partial<GitopsIssue>;
  return {
    severity: oneOf(item.severity, SEVERITIES, 'info'),
    title: item.title ?? '',
    detail: item.detail,
    subject: item.subject,
  };
}

function deploymentOf(raw: unknown): GitopsDeployment {
  const item = raw as Partial<GitopsDeployment>;
  return {
    id: item.id ?? 0,
    revision: item.revision ?? '',
    source: item.source,
    startedAt: item.startedAt,
    deployedAt: item.deployedAt,
    initiatedBy: item.initiatedBy,
    automated: item.automated,
  };
}

function operationOf(raw: unknown): GitopsOperation | undefined {
  if (raw === undefined || raw === null) {
    return undefined;
  }
  const item = raw as Partial<GitopsOperation>;
  return {
    phase: item.phase ?? '',
    running: item.running,
    message: item.message,
    cause: item.cause,
    revision: item.revision,
    startedAt: item.startedAt,
    finishedAt: item.finishedAt,
    initiatedBy: item.initiatedBy,
  };
}

export function parseGitopsApp(body: unknown, ref: ObjectRef): GitopsApp {
  const item = body as Partial<GitopsApp>;
  return {
    ref: item.ref ?? ref,
    controller: item.controller ?? '',
    terminating: item.terminating,
    kind: item.kind ?? '',
    name: item.name ?? '',
    namespace: item.namespace ?? '',
    source: {
      repo: item.source?.repo,
      path: item.source?.path,
      target: item.source?.target,
      destination: item.source?.destination,
      project: item.source?.project,
      syncMode: item.source?.syncMode ?? '',
      policy: item.source?.policy,
    },
    state: {
      sync: item.state?.sync,
      health: item.state?.health,
      revision: item.state?.revision,
      createdAt: item.state?.createdAt,
      syncedAt: item.state?.syncedAt,
      message: item.state?.message,
    },
    issues: (item.issues ?? []).map(issueOf),
    resources: (item.resources ?? []).map(resourceOf),
    history: (item.history ?? []).map(deploymentOf),
    operation: operationOf(item.operation),
    error: item.error,
  };
}

export async function fetchGitopsApp(ref: ObjectRef): Promise<GitopsApp> {
  const response = await request(`/api/gitops/app?${refQuery(ref)}`);
  if (!response.ok) {
    throw await failure(response, `the application request failed with status ${response.status}`);
  }
  return parseGitopsApp(await response.json(), ref);
}

export async function fetchGitopsAppGraph(ref: ObjectRef): Promise<Graph> {
  const response = await request(`/api/gitops/app/graph?${refQuery(ref)}`);
  if (!response.ok) {
    throw await failure(response, `the graph request failed with status ${response.status}`);
  }
  return parseGraph(await response.json());
}

export interface GitopsAppState {
  data: GitopsApp | null;
  error: string | null;
  reload: () => void;
}

export function useGitopsApp(target: ObjectRef | null, active = true): GitopsAppState {
  const [data, setData] = useState<GitopsApp | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [reloads, setReloads] = useState(0);
  const epoch = useClusterEpoch();
  let targetKey = '';
  if (target !== null) {
    targetKey = refQuery(target);
  }
  let activeKey = 'inactive';
  if (active) {
    activeKey = 'active';
  }
  const stateKey = `${String(epoch)}:${activeKey}:${targetKey}`;
  const [lastStateKey, setLastStateKey] = useState(stateKey);

  if (stateKey !== lastStateKey) {
    setLastStateKey(stateKey);
    setData(null);
    setError(null);
  }

  useEffect(() => {
    if (target === null || !active) {
      return undefined;
    }
    const wanted = target;
    let mounted = true;
    let inFlight = false;
    function load() {
      if (inFlight) {
        return;
      }
      inFlight = true;
      fetchGitopsApp(wanted)
        .then((found) => {
          if (mounted) {
            setData(found);
            setError(null);
          }
        })
        .catch((err: unknown) => {
          if (mounted) {
            setError(errorMessage(err));
          }
        })
        .finally(() => {
          inFlight = false;
        });
    }
    load();
    const timer = setInterval(load, REFRESH_MS);
    return () => {
      mounted = false;
      clearInterval(timer);
    };
  }, [target, active, reloads, epoch]);

  const reload = useCallback(() => {
    setReloads((value) => value + 1);
  }, []);

  return { data, error, reload };
}

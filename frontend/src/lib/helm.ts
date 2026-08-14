import { useCallback, useEffect, useState } from 'react';
import type {
  HelmActionResult,
  HelmReleaseDetail,
  HelmReleases,
  HelmResource,
  HelmSupport,
  ObjectRef,
} from './types';
import { request, SLOW_REQUEST_TIMEOUT_MS } from './http';
import { failure } from './object';
import {
  parseHelmActionResult,
  parseHelmReleaseDetail,
  parseHelmReleases,
  parseHelmSupport,
} from './parse';
import { usePoll } from './usePoll';
import type { Polled } from './usePoll';

const HELM_POLL_MS = 15000;

export async function fetchHelmReleases(): Promise<HelmReleases> {
  const response = await request('/api/helm');
  if (!response.ok) {
    throw await failure(response, `helm request failed with status ${response.status}`);
  }
  return parseHelmReleases(await response.json());
}

export function useHelmReleases(enabled = true): Polled<HelmReleases> {
  return usePoll(fetchHelmReleases, {
    intervalMs: HELM_POLL_MS,
    enabled,
    fallback: 'helm request failed',
  });
}

const OK_STATUSES = new Set(['deployed']);
const FAILED_STATUSES = new Set(['failed', 'unknown']);
const BUSY_STATUSES = new Set([
  'pending-install',
  'pending-upgrade',
  'pending-rollback',
  'uninstalling',
]);

export function statusTone(status: string): 'ok' | 'warn' | 'error' | 'idle' {
  if (OK_STATUSES.has(status)) {
    return 'ok';
  }
  if (FAILED_STATUSES.has(status)) {
    return 'error';
  }
  if (BUSY_STATUSES.has(status)) {
    return 'warn';
  }
  return 'idle';
}

export function statusDot(status: string): string {
  const tone = statusTone(status);
  if (tone === 'ok') {
    return 'bg-ok-solid';
  }
  if (tone === 'error') {
    return 'bg-error-solid';
  }
  if (tone === 'warn') {
    return 'bg-warn-solid';
  }
  return 'bg-idle-solid';
}

export function statusText(status: string): string {
  const tone = statusTone(status);
  if (tone === 'ok') {
    return 'text-ok';
  }
  if (tone === 'error') {
    return 'text-error';
  }
  if (tone === 'warn') {
    return 'text-warn';
  }
  return 'text-fg-muted';
}

export function statusLabel(status: string): string {
  if (status === '') {
    return 'unknown';
  }
  return status;
}

export function latestLabel(release: { latest?: string }): string {
  if (release.latest === undefined || release.latest === '') {
    return '—';
  }
  return release.latest;
}

export function latestColor(release: { outdated?: boolean }): string {
  if (release.outdated === true) {
    return 'text-warn';
  }
  return 'text-fg-muted';
}

export function latestNote(release: { latest?: string; outdated?: boolean }): string {
  if (release.latest === undefined || release.latest === '') {
    return 'no chart repository knows this chart';
  }
  if (release.outdated === true) {
    return 'a newer chart version is available';
  }
  return 'up to date';
}

export async function fetchHelmRelease(
  namespace: string,
  name: string,
): Promise<HelmReleaseDetail> {
  const params = new URLSearchParams({ namespace, name });
  const response = await request(`/api/helm/release?${params.toString()}`);
  if (!response.ok) {
    throw await failure(response, `helm release request failed with status ${response.status}`);
  }
  return parseHelmReleaseDetail(await response.json());
}

export async function fetchHelmSupport(): Promise<HelmSupport> {
  const response = await request('/api/helm/support');
  if (!response.ok) {
    throw await failure(response, `helm support request failed with status ${response.status}`);
  }
  return parseHelmSupport(await response.json());
}

export async function rollbackRelease(
  namespace: string,
  name: string,
  revision: number,
  confirm?: string,
): Promise<HelmActionResult> {
  return runAction({ namespace, name, action: 'rollback', revision: String(revision) }, confirm);
}

export async function uninstallRelease(
  namespace: string,
  name: string,
  confirm?: string,
): Promise<HelmActionResult> {
  return runAction({ namespace, name, action: 'uninstall' }, confirm);
}

async function runAction(
  params: Record<string, string>,
  confirm?: string,
): Promise<HelmActionResult> {
  const query = new URLSearchParams(params);
  if (confirm !== undefined) {
    query.set('confirm', confirm);
  }
  const response = await request(`/api/helm/action?${query.toString()}`, {
    method: 'POST',
    timeoutMs: SLOW_REQUEST_TIMEOUT_MS,
  });
  if (!response.ok) {
    throw await failure(response, `the release action failed with status ${response.status}`);
  }
  return parseHelmActionResult(await response.json());
}

export function refOf(resource: HelmResource): ObjectRef | null {
  if (resource.resource === undefined || resource.resource === '') {
    return null;
  }
  return {
    group: resource.group ?? '',
    version: resource.version ?? '',
    resource: resource.resource,
    namespace: resource.namespace ?? '',
    name: resource.name,
  };
}

export interface ReleaseDetail {
  data: HelmReleaseDetail | null;
  error: string | null;
  loading: boolean;
  reload: () => void;
}

export function useHelmRelease(namespace: string, name: string): ReleaseDetail {
  const [data, setData] = useState<HelmReleaseDetail | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [reloads, setReloads] = useState(0);

  useEffect(() => {
    if (namespace === '' || name === '') {
      setData(null);
      setError(null);
      return;
    }
    let live = true;
    setLoading(true);
    fetchHelmRelease(namespace, name)
      .then((detail) => {
        if (live) {
          setData(detail);
          setError(null);
        }
      })
      .catch((err: unknown) => {
        if (live) {
          setData(null);
          setError(messageOf(err));
        }
      })
      .finally(() => {
        if (live) {
          setLoading(false);
        }
      });
    return () => {
      live = false;
    };
  }, [namespace, name, reloads]);

  const reload = useCallback(() => {
    setReloads((count) => count + 1);
  }, []);

  return { data, error, loading, reload };
}

export function useHelmSupport(): HelmSupport | null {
  const [support, setSupport] = useState<HelmSupport | null>(null);

  useEffect(() => {
    let live = true;
    fetchHelmSupport()
      .then((found) => {
        if (live) {
          setSupport(found);
        }
      })
      .catch((err: unknown) => {
        if (live) {
          setSupport({ available: false, reason: messageOf(err), binary: 'helm' });
        }
      });
    return () => {
      live = false;
    };
  }, []);

  return support;
}

function messageOf(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'the request failed';
}

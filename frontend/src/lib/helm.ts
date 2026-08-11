import type { HelmReleases } from './types';
import { request } from './http';
import { failure } from './object';
import { parseHelmReleases } from './parse';
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

import type { DebugSession, DebugSupport, ExecTarget } from './types';
import { failure } from './object';
import { request, SLOW_REQUEST_TIMEOUT_MS } from './http';
import { parseDebugSession, parseDebugSupport } from './parse';
import { execQuery } from './exec';

export const DEBUG_PROFILES = [
  'general',
  'netadmin',
  'sysadmin',
  'baseline',
  'restricted',
] as const;

export type DebugProfile = (typeof DEBUG_PROFILES)[number];

export const DEFAULT_PROFILE: DebugProfile = 'general';

export async function startDebug(target: ExecTarget, profile: DebugProfile): Promise<DebugSession> {
  const response = await request(`/api/debug?${execQuery(target)}&profile=${profile}`, {
    method: 'POST',
    timeoutMs: SLOW_REQUEST_TIMEOUT_MS,
  });
  if (!response.ok) {
    throw await failure(
      response,
      `starting a debug container failed with status ${response.status}`,
    );
  }
  return parseDebugSession(await response.json());
}

export async function fetchDebugSupport(namespace: string, pod: string): Promise<DebugSupport> {
  const params = new URLSearchParams({ namespace, pod });
  const response = await request(`/api/debug/support?${params.toString()}`);
  if (!response.ok) {
    throw await failure(response, `debug support failed with status ${response.status}`);
  }
  return parseDebugSupport(await response.json());
}

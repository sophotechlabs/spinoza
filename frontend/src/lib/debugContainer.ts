import type { DebugSession, DebugSupport, ExecTarget } from './types';
import { failure } from './object';
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
  const response = await fetch(`/api/debug?${execQuery(target)}&profile=${profile}`, {
    method: 'POST',
  });
  if (!response.ok) {
    throw await failure(
      response,
      `starting a debug container failed with status ${response.status}`,
    );
  }
  return (await response.json()) as DebugSession;
}

export async function fetchDebugSupport(namespace: string, pod: string): Promise<DebugSupport> {
  const params = new URLSearchParams({ namespace, pod });
  const response = await fetch(`/api/debug/support?${params.toString()}`);
  if (!response.ok) {
    throw await failure(response, `debug support failed with status ${response.status}`);
  }
  return (await response.json()) as DebugSupport;
}

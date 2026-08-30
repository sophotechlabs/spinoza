import type { RBACGrant, RBACIndex, RBACRule, RBACSubject } from './types';
import { request } from './http';
import { failure } from './object';
import { usePoll } from './usePoll';
import type { Polled } from './usePoll';

const RBAC_POLL_MS = 30000;

export interface Ask {
  verb: string;
  group: string;
  resource: string;
  namespace: string;
}

export const NO_ASK: Ask = { verb: '', group: '', resource: '', namespace: '' };

async function readIndex(path: string, what: string): Promise<RBACIndex> {
  const response = await request(path);
  if (!response.ok) {
    throw await failure(response, `${what} failed with status ${response.status}`);
  }
  const body = (await response.json()) as Partial<RBACIndex>;
  return {
    subjects: body.subjects ?? [],
    absent: body.absent,
    dropped: body.dropped,
    error: body.error,
  };
}

export async function fetchRBAC(): Promise<RBACIndex> {
  return readIndex('/api/rbac', 'the permission index');
}

export function askable(ask: Ask): boolean {
  return ask.verb.trim() !== '' && ask.resource.trim() !== '';
}

export async function fetchWho(ask: Ask): Promise<RBACIndex> {
  const params = new URLSearchParams({ verb: ask.verb.trim(), resource: ask.resource.trim() });
  if (ask.group.trim() !== '') {
    params.set('group', ask.group.trim());
  }
  if (ask.namespace.trim() !== '') {
    params.set('namespace', ask.namespace.trim());
  }
  return readIndex(`/api/rbac/who?${params.toString()}`, 'the question');
}

export function useRBAC(enabled = true): Polled<RBACIndex> {
  return usePoll(fetchRBAC, {
    intervalMs: RBAC_POLL_MS,
    enabled,
    fallback: 'the permission index failed',
  });
}

// A subject bound everywhere has no namespace list, and saying "everywhere" is
// clearer than saying nothing.
export function whereLabel(subject: RBACSubject): string {
  const held = subject.namespaces ?? [];
  if (held.length === 0) {
    return 'everywhere';
  }
  if (held.length <= 3) {
    return held.join(', ');
  }
  return `${held.slice(0, 3).join(', ')} and ${String(held.length - 3)} more`;
}

// A grant reads as the binding that made it, because that is the object a
// person edits to take it away.
export function grantLabel(grant: RBACGrant): string {
  const where =
    grant.namespace === undefined || grant.namespace === '' ? 'everywhere' : grant.namespace;
  return `${grant.bindingKind} ${grant.binding} → ${grant.roleKind} ${grant.role} · ${where}`;
}

export function ruleLabel(rule: RBACRule): string {
  const verbs = (rule.verbs ?? []).join(', ');
  const resources = (rule.resources ?? []).join(', ');
  const groups = (rule.groups ?? []).filter((one) => one !== '').join(', ');
  const parts = [verbs === '' ? 'nothing' : verbs, 'on', resources === '' ? 'nothing' : resources];
  if (groups !== '') {
    parts.push(`in ${groups}`);
  }
  if ((rule.names ?? []).length > 0) {
    parts.push(`named ${(rule.names ?? []).join(', ')}`);
  }
  return parts.join(' ');
}

// An aggregated role with no rules yet is waiting on a controller, not
// harmless, and the row should not read as empty.
export function grantNote(grant: RBACGrant): string {
  if (grant.missing === true) {
    return 'the role it names does not exist';
  }
  if (grant.aggregated === true && (grant.rules ?? []).length === 0) {
    return 'aggregated, and the controller has not filled it in yet';
  }
  return '';
}

export function whoFailure(err: unknown): string {
  if (err instanceof Error) {
    return `Asking who can: ${err.message}`;
  }
  return 'Asking who can failed';
}

export function matches(subject: RBACSubject, query: string): boolean {
  const wanted = query.trim().toLowerCase();
  if (wanted === '') {
    return true;
  }
  if (subject.label.toLowerCase().includes(wanted)) {
    return true;
  }
  return (subject.powers ?? []).some((one) => one.toLowerCase().includes(wanted));
}

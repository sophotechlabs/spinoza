import type { Scope, Session } from './types';
import { request } from './http';

const SESSION_PATH = '/api/auth/me';

export const SIGN_OUT_PATH = '/auth/logout';

const SIGN_IN_PATH = '/auth/login';

interface WireScope {
  everywhere?: boolean;
  namespaces?: string[];
  undecided?: string[];
}

interface WireSession {
  authenticated?: boolean;
  cluster?: boolean;
  error?: string;
  groups?: string[];
  mode?: string;
  role?: string;
  scope?: WireScope;
  signIn?: boolean;
  user?: string;
}

export const OWN_WINDOW: Session = {
  authenticated: true,
  cluster: false,
  mode: 'none',
  role: 'admin',
  scope: { everywhere: true },
  signIn: false,
};

function normalizeScope(scope: WireScope | undefined): Scope {
  if (scope === undefined) {
    return { everywhere: true };
  }
  if (scope.everywhere === true) {
    return { everywhere: true };
  }
  return {
    everywhere: false,
    namespaces: scope.namespaces ?? [],
    undecided: scope.undecided ?? [],
  };
}

export function normalizeSession(body: WireSession): Session {
  return {
    authenticated: body.authenticated ?? false,
    cluster: body.cluster ?? false,
    error: body.error,
    groups: body.groups,
    mode: body.mode ?? 'none',
    role: body.role ?? '',
    scope: normalizeScope(body.scope),
    signIn: body.signIn ?? false,
    user: body.user,
  };
}

export async function fetchSession(): Promise<Session> {
  const response = await request(SESSION_PATH);
  if (!response.ok) {
    return OWN_WINDOW;
  }
  return normalizeSession((await response.json()) as WireSession);
}

export function signInHref(): string {
  const here = window.location.pathname + window.location.search + window.location.hash;
  const params = new URLSearchParams({ next: here });
  return `${SIGN_IN_PATH}?${params.toString()}`;
}

export function signedOut(session: Session): boolean {
  if (!session.cluster) {
    return false;
  }
  return !session.authenticated;
}

function readableNote(names: string[]): string {
  if (names.length === 0) {
    return 'no namespace';
  }
  if (names.length === 1) {
    return `the ${names[0]} namespace`;
  }
  return `${String(names.length)} namespaces`;
}

function undecidedNote(names: string[]): string {
  if (names.length === 0) {
    return '';
  }
  return `, and ${String(names.length)} the cluster would not decide`;
}

export function scopeSummary(scope: Scope): string {
  if (scope.everywhere) {
    return 'every namespace';
  }
  return readableNote(scope.namespaces) + undecidedNote(scope.undecided);
}

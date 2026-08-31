import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  OWN_WINDOW,
  fetchSession,
  holds,
  mayAdminister,
  mayEdit,
  normalizeSession,
  scopeSummary,
  signInHref,
  signedOut,
} from '../../src/lib/identity';
import type { Session } from '../../src/lib/types';
import { fetchOverview } from '../../src/lib/overview';

function stub(body: unknown, ok = true, status = 200) {
  const fetchMock = vi.fn((url: string, init?: RequestInit) => {
    void init;
    void url;
    return Promise.resolve({ ok, status, json: () => Promise.resolve(body) });
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

function session(overrides: Partial<Session>): Session {
  return { ...OWN_WINDOW, ...overrides };
}

afterEach(() => {
  vi.unstubAllGlobals();
  window.history.replaceState(null, '', '/');
});

describe('normalizeSession', () => {
  it('fills in everything the backend left out', () => {
    expect(normalizeSession({})).toEqual({
      authenticated: false,
      cluster: false,
      error: undefined,
      groups: undefined,
      mode: 'none',
      role: '',
      scope: { everywhere: true },
      signIn: false,
      user: undefined,
    });
  });

  it('keeps what the backend did say', () => {
    const found = normalizeSession({
      authenticated: true,
      cluster: true,
      groups: ['platform'],
      mode: 'oidc',
      role: 'editor',
      scope: { everywhere: false, namespaces: ['payments'] },
      signIn: true,
      user: 'alice@example.com',
    });

    expect(found.user).toBe('alice@example.com');
    expect(found.scope).toEqual({ everywhere: false, namespaces: ['payments'], undecided: [] });
  });

  it('reads a scope that names no namespaces as reaching none of them', () => {
    expect(normalizeSession({ scope: { everywhere: false } }).scope).toEqual({
      everywhere: false,
      namespaces: [],
      undecided: [],
    });
  });

  it('carries the namespaces the cluster would not decide about', () => {
    expect(
      normalizeSession({ scope: { everywhere: false, namespaces: ['a'], undecided: ['b'] } }).scope,
    ).toEqual({ everywhere: false, namespaces: ['a'], undecided: ['b'] });
  });
});

describe('fetchSession', () => {
  it('asks the backend who is signed in', async () => {
    const fetchMock = stub({ authenticated: true, cluster: true, mode: 'oidc', role: 'admin' });

    const found = await fetchSession();

    expect(fetchMock.mock.calls[0][0]).toBe('/api/auth/me');
    expect(found.role).toBe('admin');
  });

  it('treats a backend that will not answer as your own window', async () => {
    stub({}, false, 500);

    expect(await fetchSession()).toEqual(OWN_WINDOW);
  });
});

describe('signInHref', () => {
  it('carries where you were, so signing in lands you back there', () => {
    window.history.replaceState(null, '', '/?view=checks#top');

    expect(signInHref()).toBe('/auth/login?next=%2F%3Fview%3Dchecks%23top');
  });
});

describe('roles', () => {
  it('lets a stronger role do what a weaker one may', () => {
    expect(holds(session({ role: 'admin' }), 'editor')).toBe(true);
    expect(mayEdit(session({ role: 'editor' }))).toBe(true);
    expect(mayAdminister(session({ role: 'editor' }))).toBe(false);
  });

  it('refuses a role it does not know', () => {
    expect(holds(session({ role: 'nobody' }), 'viewer')).toBe(false);
  });
});

describe('signedOut', () => {
  it('is never true for a window you started yourself', () => {
    expect(signedOut(session({ cluster: false, authenticated: false }))).toBe(false);
  });

  it('is true for a served spinoza nobody has signed in to', () => {
    expect(signedOut(session({ cluster: true, authenticated: false }))).toBe(true);
  });
});

describe('scopeSummary', () => {
  it('says every namespace when nothing is held back', () => {
    expect(scopeSummary({ everywhere: true })).toBe('every namespace');
  });

  it('names the one namespace you can read', () => {
    expect(scopeSummary({ everywhere: false, namespaces: ['payments'], undecided: [] })).toBe(
      'the payments namespace',
    );
  });

  it('counts them once there is more than one', () => {
    expect(scopeSummary({ everywhere: false, namespaces: ['a', 'b'], undecided: [] })).toBe(
      '2 namespaces',
    );
  });

  it('says so when you can read none of them', () => {
    expect(scopeSummary({ everywhere: false, namespaces: [], undecided: [] })).toBe('no namespace');
  });

  it('does not render an undecided namespace as one you cannot read', () => {
    const answered = scopeSummary({ everywhere: false, namespaces: ['a', 'b'], undecided: [] });
    const partial = scopeSummary({
      everywhere: false,
      namespaces: ['a', 'b'],
      undecided: ['payments'],
    });

    expect(partial).not.toBe(answered);
    expect(partial).toBe('2 namespaces, and 1 the cluster would not decide');
  });

  it('keeps the undecided ones off the readable list', () => {
    expect(
      scopeSummary({ everywhere: false, namespaces: [], undecided: ['payments', 'billing'] }),
    ).toBe('no namespace, and 2 the cluster would not decide');
  });
});

describe('a view that reads the whole cluster', () => {
  it('shows the reason the backend gave, not the bare status', async () => {
    stub(
      {
        message: 'this view reads the whole cluster, and your account reads named namespaces only',
      },
      false,
      403,
    );

    await expect(fetchOverview()).rejects.toThrow(/reads the whole cluster/);
  });
});

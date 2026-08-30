import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  NO_ASK,
  askable,
  fetchRBAC,
  fetchWho,
  grantLabel,
  grantNote,
  matches,
  ruleLabel,
  whereLabel,
  whoFailure,
} from '../../src/lib/rbac';
import type { RBACGrant, RBACSubject } from '../../src/lib/types';

function stub(body: unknown, ok = true, status = 200) {
  const fetcher = vi.fn((url: string) => {
    void url;
    return Promise.resolve({ ok, status, json: () => Promise.resolve(body) });
  });
  vi.stubGlobal('fetch', fetcher);
  return fetcher;
}

function subject(extra: Partial<RBACSubject> = {}): RBACSubject {
  return { kind: 'User', name: 'ana', label: 'ana', grants: [], ...extra };
}

function grant(extra: Partial<RBACGrant> = {}): RBACGrant {
  return {
    binding: 'read',
    bindingKind: 'ClusterRoleBinding',
    role: 'reader',
    roleKind: 'ClusterRole',
    ...extra,
  };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('reading the index', () => {
  it('reads the subjects back', async () => {
    stub({ subjects: [subject()] });

    expect((await fetchRBAC()).subjects).toHaveLength(1);
  });

  it('fills in an empty answer', async () => {
    stub({});

    expect((await fetchRBAC()).subjects).toEqual([]);
  });

  it('reports a failure with its status', async () => {
    stub({ message: 'no' }, false, 500);

    await expect(fetchRBAC()).rejects.toThrow();
  });
});

describe('asking who can', () => {
  it('needs a verb and a resource', () => {
    expect(askable(NO_ASK)).toBe(false);
    expect(askable({ ...NO_ASK, verb: 'create' })).toBe(false);
    expect(askable({ ...NO_ASK, verb: 'create', resource: 'pods' })).toBe(true);
  });

  it('treats blank space as nothing asked', () => {
    expect(askable({ ...NO_ASK, verb: '  ', resource: ' ' })).toBe(false);
  });

  it('carries the verb and resource', async () => {
    const fetcher = stub({ subjects: [] });

    await fetchWho({ ...NO_ASK, verb: 'create', resource: 'pods/exec' });

    expect(fetcher.mock.calls[0][0]).toContain('verb=create');
    expect(fetcher.mock.calls[0][0]).toContain('pods%2Fexec');
  });

  it('leaves out the group and namespace when they are blank', async () => {
    const fetcher = stub({ subjects: [] });

    await fetchWho({ ...NO_ASK, verb: 'get', resource: 'secrets' });

    expect(fetcher.mock.calls[0][0]).not.toContain('group=');
    expect(fetcher.mock.calls[0][0]).not.toContain('namespace=');
  });

  it('carries the group and namespace when they are given', async () => {
    const fetcher = stub({ subjects: [] });

    await fetchWho({ verb: 'create', resource: 'deployments', group: 'apps', namespace: 'web' });

    expect(fetcher.mock.calls[0][0]).toContain('group=apps');
    expect(fetcher.mock.calls[0][0]).toContain('namespace=web');
  });

  it('reports a failure', async () => {
    stub({}, false, 400);

    await expect(fetchWho({ ...NO_ASK, verb: 'get', resource: 'pods' })).rejects.toThrow();
  });
});

describe('when the question itself fails', () => {
  it('says what failed and why', async () => {
    stub({ message: 'no' }, false, 400);

    await expect(fetchWho({ ...NO_ASK, verb: 'get', resource: 'pods' })).rejects.toThrow();
  });
});

describe('what a row says', () => {
  it('says everywhere for a subject with no namespaces', () => {
    expect(whereLabel(subject())).toBe('everywhere');
  });

  it('lists a few namespaces', () => {
    expect(whereLabel(subject({ namespaces: ['db', 'web'] }))).toBe('db, web');
  });

  it('counts the rest when there are many', () => {
    expect(whereLabel(subject({ namespaces: ['a', 'b', 'c', 'd', 'e'] }))).toBe(
      'a, b, c and 2 more',
    );
  });

  it('names the binding that made a grant', () => {
    expect(grantLabel(grant())).toBe('ClusterRoleBinding read → ClusterRole reader · everywhere');
  });

  it('names the namespace a grant is confined to', () => {
    expect(grantLabel(grant({ namespace: 'web' }))).toContain('· web');
  });

  it('says when a grant names a role that is not there', () => {
    expect(grantNote(grant({ missing: true }))).toBe('the role it names does not exist');
  });

  it('says when an aggregated role has not been filled in', () => {
    expect(grantNote(grant({ aggregated: true, rules: [] }))).toContain('controller');
  });

  it('says nothing about an ordinary grant', () => {
    expect(grantNote(grant({ rules: [{ verbs: ['get'] }] }))).toBe('');
  });

  it('reads a rule as verbs on resources', () => {
    expect(ruleLabel({ verbs: ['get', 'list'], resources: ['pods'], groups: [''] })).toBe(
      'get, list on pods',
    );
  });

  it('names the api group when there is one', () => {
    expect(ruleLabel({ verbs: ['get'], resources: ['deployments'], groups: ['apps'] })).toBe(
      'get on deployments in apps',
    );
  });

  it('says when a rule is limited to named objects', () => {
    expect(ruleLabel({ verbs: ['get'], resources: ['secrets'], names: ['token'] })).toContain(
      'named token',
    );
  });

  it('says nothing rather than blank for an empty rule', () => {
    expect(ruleLabel({})).toBe('nothing on nothing');
  });
});

describe('when the question fails', () => {
  it('says what failed and why', () => {
    expect(whoFailure(new Error('the apiserver refused'))).toBe(
      'Asking who can: the apiserver refused',
    );
  });

  it('says what failed when there is no why', () => {
    expect(whoFailure('nope')).toBe('Asking who can failed');
  });
});

describe('filtering the list', () => {
  it('keeps everything for an empty query', () => {
    expect(matches(subject(), '  ')).toBe(true);
  });

  it('matches on the subject name', () => {
    expect(matches(subject({ label: 'system:serviceaccount:web:api' }), 'web')).toBe(true);
  });

  it('matches on a power', () => {
    expect(matches(subject({ powers: ['reads secrets'] }), 'secret')).toBe(true);
  });

  it('drops what does not match', () => {
    expect(matches(subject(), 'zzz')).toBe(false);
  });
});

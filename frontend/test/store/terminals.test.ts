import { beforeEach, describe, expect, it } from 'vitest';
import {
  LOCAL_SESSION,
  clearTerminals,
  forgetTerminals,
  sessionId,
  terminalsNow,
  useTerminalsStore,
} from '../../src/store/terminals';
import { MK1, MK2, showing } from '../helpers-clusters';

function state() {
  return useTerminalsStore.getState();
}

function ids(cluster = MK1): string[] {
  return (state().byCluster[cluster]?.sessions ?? []).map((session) => session.id);
}

function activeOn(cluster = MK1): string | null {
  return state().byCluster[cluster]?.active ?? null;
}

beforeEach(() => {
  clearTerminals();
  showing(MK1);
});

describe('terminal sessions', () => {
  it('starts with nothing open', () => {
    expect(terminalsNow()).toHaveLength(0);
    expect(activeOn()).toBeNull();
  });

  it('opens a shell and brings it to the front', () => {
    state().open('prod', 'web', 'app');

    expect(ids()).toEqual([sessionId('prod', 'web', 'app')]);
    expect(activeOn()).toBe(sessionId('prod', 'web', 'app'));
    expect(terminalsNow()[0]).toMatchObject({ namespace: 'prod', pod: 'web', container: 'app' });
  });

  it('keeps one shell per pod and container', () => {
    state().open('prod', 'web', 'app');
    state().open('prod', 'db', 'psql');
    state().open('prod', 'web', 'app');

    expect(ids()).toHaveLength(2);
    expect(activeOn()).toBe(sessionId('prod', 'web', 'app'));
  });

  it('tells apart the same pod name in two namespaces', () => {
    state().open('prod', 'web', 'app');
    state().open('staging', 'web', 'app');

    expect(ids()).toHaveLength(2);
  });

  it('brings an older shell back to the front', () => {
    state().open('prod', 'web', 'app');
    state().open('prod', 'db', 'psql');

    state().focus(sessionId('prod', 'web', 'app'));

    expect(activeOn()).toBe(sessionId('prod', 'web', 'app'));
  });

  it('closes a shell and falls back to the one left', () => {
    state().open('prod', 'web', 'app');
    state().open('prod', 'db', 'psql');

    state().close(sessionId('prod', 'db', 'psql'));

    expect(ids()).toEqual([sessionId('prod', 'web', 'app')]);
    expect(activeOn()).toBe(sessionId('prod', 'web', 'app'));
  });

  it('leaves the front shell alone when another one closes', () => {
    state().open('prod', 'web', 'app');
    state().open('prod', 'db', 'psql');

    state().close(sessionId('prod', 'web', 'app'));

    expect(activeOn()).toBe(sessionId('prod', 'db', 'psql'));
  });

  it('has nothing in front once the last shell closes', () => {
    state().open('prod', 'web', 'app');

    state().close(sessionId('prod', 'web', 'app'));

    expect(terminalsNow()).toHaveLength(0);
    expect(activeOn()).toBeNull();
  });

  it('drops every shell when the cluster changes', () => {
    state().open('prod', 'web', 'app');
    state().open('prod', 'db', 'psql');

    clearTerminals();

    expect(terminalsNow()).toHaveLength(0);
    expect(activeOn()).toBeNull();
  });

  it('opens a shell on this machine and puts it first', () => {
    state().open('prod', 'web', 'app');

    state().openLocal();

    expect(ids()).toEqual([LOCAL_SESSION, sessionId('prod', 'web', 'app')]);
    expect(activeOn()).toBe(LOCAL_SESSION);
    expect(terminalsNow()[0].kind).toBe('local');
  });

  it('keeps one shell on this machine, not several', () => {
    state().openLocal();
    state().open('prod', 'web', 'app');

    state().openLocal();

    expect(ids()).toHaveLength(2);
    expect(activeOn()).toBe(LOCAL_SESSION);
  });

  it('marks pod shells as such', () => {
    state().open('prod', 'web', 'app');

    expect(terminalsNow()[0].kind).toBe('pod');
  });
});

describe('a shell on a node', () => {
  it('opens one session per node and focuses it', () => {
    useTerminalsStore.getState().openNode('p-mk1');

    expect(terminalsNow()).toHaveLength(1);
    expect(terminalsNow()[0]).toMatchObject({ kind: 'node', pod: 'p-mk1', id: 'node/p-mk1' });
    expect(activeOn()).toBe('node/p-mk1');
  });

  it('focuses the one it already has rather than opening a second', () => {
    useTerminalsStore.getState().openNode('p-mk1');
    useTerminalsStore.getState().openNode('p-mk2');
    useTerminalsStore.getState().openNode('p-mk1');

    expect(terminalsNow()).toHaveLength(2);
    expect(activeOn()).toBe('node/p-mk1');
  });
});

describe('terminals on another tab', () => {
  beforeEach(() => {
    clearTerminals();
    showing(MK1);
  });

  it('belong to the cluster they were opened on', () => {
    state().open('prod', 'web', 'app');

    showing(MK2);
    state().open('prod', 'api', 'app');

    expect(ids()).toEqual([sessionId('prod', 'web', 'app')]);
    expect(ids(MK2)).toEqual([sessionId('prod', 'api', 'app')]);
  });

  it('go when the tab is closed', () => {
    state().open('prod', 'web', 'app');

    forgetTerminals(MK1);

    expect(ids()).toEqual([]);
    expect(activeOn()).toBeNull();
  });
});

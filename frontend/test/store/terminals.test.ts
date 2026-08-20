import { beforeEach, describe, expect, it } from 'vitest';
import {
  LOCAL_SESSION,
  clearTerminals,
  sessionId,
  useTerminalsStore,
} from '../../src/store/terminals';

function state() {
  return useTerminalsStore.getState();
}

function ids(): string[] {
  return state().sessions.map((session) => session.id);
}

beforeEach(() => {
  clearTerminals();
});

describe('terminal sessions', () => {
  it('starts with nothing open', () => {
    expect(state().sessions).toHaveLength(0);
    expect(state().active).toBeNull();
  });

  it('opens a shell and brings it to the front', () => {
    state().open('prod', 'web', 'app');

    expect(ids()).toEqual([sessionId('prod', 'web', 'app')]);
    expect(state().active).toBe(sessionId('prod', 'web', 'app'));
    expect(state().sessions[0]).toMatchObject({ namespace: 'prod', pod: 'web', container: 'app' });
  });

  it('keeps one shell per pod and container', () => {
    state().open('prod', 'web', 'app');
    state().open('prod', 'db', 'psql');
    state().open('prod', 'web', 'app');

    expect(ids()).toHaveLength(2);
    expect(state().active).toBe(sessionId('prod', 'web', 'app'));
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

    expect(state().active).toBe(sessionId('prod', 'web', 'app'));
  });

  it('closes a shell and falls back to the one left', () => {
    state().open('prod', 'web', 'app');
    state().open('prod', 'db', 'psql');

    state().close(sessionId('prod', 'db', 'psql'));

    expect(ids()).toEqual([sessionId('prod', 'web', 'app')]);
    expect(state().active).toBe(sessionId('prod', 'web', 'app'));
  });

  it('leaves the front shell alone when another one closes', () => {
    state().open('prod', 'web', 'app');
    state().open('prod', 'db', 'psql');

    state().close(sessionId('prod', 'web', 'app'));

    expect(state().active).toBe(sessionId('prod', 'db', 'psql'));
  });

  it('has nothing in front once the last shell closes', () => {
    state().open('prod', 'web', 'app');

    state().close(sessionId('prod', 'web', 'app'));

    expect(state().sessions).toHaveLength(0);
    expect(state().active).toBeNull();
  });

  it('drops every shell when the cluster changes', () => {
    state().open('prod', 'web', 'app');
    state().open('prod', 'db', 'psql');

    clearTerminals();

    expect(state().sessions).toHaveLength(0);
    expect(state().active).toBeNull();
  });

  it('opens a shell on this machine and puts it first', () => {
    state().open('prod', 'web', 'app');

    state().openLocal();

    expect(ids()).toEqual([LOCAL_SESSION, sessionId('prod', 'web', 'app')]);
    expect(state().active).toBe(LOCAL_SESSION);
    expect(state().sessions[0].kind).toBe('local');
  });

  it('keeps one shell on this machine, not several', () => {
    state().openLocal();
    state().open('prod', 'web', 'app');

    state().openLocal();

    expect(ids()).toHaveLength(2);
    expect(state().active).toBe(LOCAL_SESSION);
  });

  it('marks pod shells as such', () => {
    state().open('prod', 'web', 'app');

    expect(state().sessions[0].kind).toBe('pod');
  });
});

describe('a shell on a node', () => {
  it('opens one session per node and focuses it', () => {
    useTerminalsStore.getState().openNode('p-mk1');

    const state = useTerminalsStore.getState();
    expect(state.sessions).toHaveLength(1);
    expect(state.sessions[0]).toMatchObject({ kind: 'node', pod: 'p-mk1', id: 'node/p-mk1' });
    expect(state.active).toBe('node/p-mk1');
  });

  it('focuses the one it already has rather than opening a second', () => {
    useTerminalsStore.getState().openNode('p-mk1');
    useTerminalsStore.getState().openNode('p-mk2');
    useTerminalsStore.getState().openNode('p-mk1');

    const state = useTerminalsStore.getState();
    expect(state.sessions).toHaveLength(2);
    expect(state.active).toBe('node/p-mk1');
  });
});

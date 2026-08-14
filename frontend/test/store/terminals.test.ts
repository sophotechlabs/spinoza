import { beforeEach, describe, expect, it } from 'vitest';
import { clearTerminals, sessionId, useTerminalsStore } from '../../src/store/terminals';

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
});

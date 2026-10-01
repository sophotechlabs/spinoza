import { afterEach, describe, expect, it } from 'vitest';
import { OWN_WINDOW } from '../../src/lib/identity';
import { isSaving, startSaving, stopSaving } from '../../src/lib/persist';
import {
  adoptSession,
  useClusterMode,
  useIdentityStore,
  useSessionKnown,
} from '../../src/store/identity';
import { renderHook } from '@testing-library/react';

describe('the identity store', () => {
  it('starts as your own window, with nothing asked yet', () => {
    expect(useIdentityStore.getState().session).toEqual(OWN_WINDOW);
    expect(renderHook(() => useSessionKnown()).result.current).toBe(false);
  });

  it('remembers what the backend said', () => {
    adoptSession({ ...OWN_WINDOW, cluster: true, user: 'alice', role: 'viewer' });

    expect(useIdentityStore.getState().session.user).toBe('alice');
    expect(renderHook(() => useClusterMode()).result.current).toBe(true);
    expect(useIdentityStore.getState().known).toBe(true);
  });
});

describe('whose settings get saved', () => {
  afterEach(() => {
    stopSaving();
  });

  it('stops saving in a served cluster where nobody signed in', () => {
    startSaving();

    adoptSession({ ...OWN_WINDOW, cluster: true, user: undefined, role: 'admin' });

    expect(isSaving()).toBe(false);
  });

  it('stops saving when the served session names an empty user', () => {
    startSaving();

    adoptSession({ ...OWN_WINDOW, cluster: true, user: '', role: 'admin' });

    expect(isSaving()).toBe(false);
  });

  it('keeps saving for a signed-in person', () => {
    startSaving();

    adoptSession({ ...OWN_WINDOW, cluster: true, user: 'alice', role: 'viewer' });

    expect(isSaving()).toBe(true);
  });

  it('keeps saving in your own window', () => {
    startSaving();

    adoptSession(OWN_WINDOW);

    expect(isSaving()).toBe(true);
  });
});

import { describe, expect, it } from 'vitest';
import { OWN_WINDOW } from '../../src/lib/identity';
import {
  adoptSession,
  currentSession,
  useClusterMode,
  useIdentityStore,
  useSessionKnown,
} from '../../src/store/identity';
import { renderHook } from '@testing-library/react';

describe('the identity store', () => {
  it('starts as your own window, with nothing asked yet', () => {
    expect(currentSession()).toEqual(OWN_WINDOW);
    expect(renderHook(() => useSessionKnown()).result.current).toBe(false);
  });

  it('remembers what the backend said', () => {
    adoptSession({ ...OWN_WINDOW, cluster: true, user: 'alice', role: 'viewer' });

    expect(currentSession().user).toBe('alice');
    expect(renderHook(() => useClusterMode()).result.current).toBe(true);
    expect(useIdentityStore.getState().known).toBe(true);
  });
});

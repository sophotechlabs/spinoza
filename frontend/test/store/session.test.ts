import { describe, expect, it } from 'vitest';
import { expireSession, sessionExpired, useSessionStore } from '../../src/store/session';

describe('the session behind this page', () => {
  it('starts out usable', () => {
    expect(sessionExpired()).toBe(false);
  });

  it('is expired once something answered 401', () => {
    expireSession();

    expect(sessionExpired()).toBe(true);
    expect(useSessionStore.getState().expired).toBe(true);
  });

  it('does not churn the store once it is already expired', () => {
    expireSession();
    let renders = 0;
    const stop = useSessionStore.subscribe(() => {
      renders += 1;
    });

    expireSession();
    stop();

    expect(renders).toBe(0);
  });

  it('can be handed back a working token', () => {
    expireSession();

    useSessionStore.getState().reset();

    expect(sessionExpired()).toBe(false);
  });
});

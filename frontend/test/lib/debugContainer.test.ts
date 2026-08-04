import { afterEach, describe, expect, it, vi } from 'vitest';
import { DEBUG_PROFILES, DEFAULT_PROFILE, startDebug } from '../../src/lib/debugContainer';
import type { ExecTarget } from '../../src/lib/types';
import { anySignal } from '../helpers';

const target: ExecTarget = { namespace: 'monitoring', pod: 'loki-0', container: 'loki' };

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('startDebug', () => {
  it('posts the target and profile and returns the session', async () => {
    const session = {
      container: 'spinoza-debug-1',
      created: true,
      image: 'busybox:1.37',
      profile: 'general',
    };
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(session) });
    vi.stubGlobal('fetch', fetchMock);

    await expect(startDebug(target, 'general')).resolves.toEqual(session);
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/debug?namespace=monitoring&pod=loki-0&container=loki&profile=general',
      { method: 'POST', signal: anySignal() },
    );
  });

  it('surfaces the server message', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 403,
        json: () => Promise.resolve({ message: 'cannot patch ephemeralcontainers' }),
      }),
    );

    await expect(startDebug(target, 'general')).rejects.toThrow('cannot patch ephemeralcontainers');
  });

  it('falls back to a status message', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 500,
        json: () => Promise.reject(new Error('not json')),
      }),
    );

    await expect(startDebug(target, 'sysadmin')).rejects.toThrow(
      'starting a debug container failed with status 500',
    );
  });

  it('defaults to the non-privileged profile', () => {
    expect(DEFAULT_PROFILE).toBe('general');
    expect(DEBUG_PROFILES[0]).toBe('general');
  });
});

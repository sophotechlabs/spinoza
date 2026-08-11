import { afterEach, describe, expect, it, vi } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import {
  fetchHelmReleases,
  statusDot,
  statusLabel,
  statusText,
  statusTone,
  useHelmReleases,
} from '../../src/lib/helm';
import { anySignal } from '../helpers';

const payload = {
  releases: [
    {
      name: 'podinfo',
      namespace: 'demo',
      chart: 'podinfo',
      chartVersion: '6.9.2',
      appVersion: '6.9.2',
      revision: 3,
      status: 'deployed',
      updated: '2026-08-11T09:30:00Z',
      description: 'Upgrade complete',
    },
  ],
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('fetchHelmReleases', () => {
  it('requests /api/helm and parses what comes back', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(payload) });
    vi.stubGlobal('fetch', fetchMock);

    const got = await fetchHelmReleases();

    expect(fetchMock).toHaveBeenCalledWith('/api/helm', { signal: anySignal() });
    expect(got.releases).toHaveLength(1);
    expect(got.releases[0].chartVersion).toBe('6.9.2');
    expect(got.releases[0].revision).toBe(3);
  });

  it('reads a payload with no releases at all', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({}) }),
    );

    const got = await fetchHelmReleases();

    expect(got.releases).toEqual([]);
    expect(got.error).toBeUndefined();
  });

  it('carries the server message when the request is refused', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 403,
        json: () => Promise.resolve({ message: 'secrets is forbidden' }),
      }),
    );

    await expect(fetchHelmReleases()).rejects.toThrow('secrets is forbidden');
  });

  it('falls back to the status when the body says nothing', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue({ ok: false, status: 500, json: () => Promise.reject(new Error()) }),
    );

    await expect(fetchHelmReleases()).rejects.toThrow('helm request failed with status 500');
  });
});

describe('useHelmReleases', () => {
  it('polls once the view is mounted', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(payload) }),
    );

    const { result } = renderHook(() => useHelmReleases());

    await waitFor(() => {
      expect(result.current.data?.releases).toHaveLength(1);
    });
  });

  it('stays quiet while the view is hidden', () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    renderHook(() => useHelmReleases(false));

    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe('what a release status looks like', () => {
  it('reads deployed as healthy', () => {
    expect(statusTone('deployed')).toBe('ok');
    expect(statusDot('deployed')).toBe('bg-ok-solid');
    expect(statusText('deployed')).toBe('text-ok');
  });

  it('reads failed and unknown as failures', () => {
    expect(statusTone('failed')).toBe('error');
    expect(statusTone('unknown')).toBe('error');
    expect(statusDot('failed')).toBe('bg-error-solid');
    expect(statusText('failed')).toBe('text-error');
  });

  it('reads anything pending as busy', () => {
    expect(statusTone('pending-upgrade')).toBe('warn');
    expect(statusTone('uninstalling')).toBe('warn');
    expect(statusDot('pending-install')).toBe('bg-warn-solid');
    expect(statusText('pending-rollback')).toBe('text-warn');
  });

  it('reads a superseded or unset status as idle', () => {
    expect(statusTone('superseded')).toBe('idle');
    expect(statusTone('')).toBe('idle');
    expect(statusDot('superseded')).toBe('bg-idle-solid');
    expect(statusText('superseded')).toBe('text-fg-muted');
  });

  it('names an empty status rather than showing a gap', () => {
    expect(statusLabel('')).toBe('unknown');
    expect(statusLabel('deployed')).toBe('deployed');
  });
});

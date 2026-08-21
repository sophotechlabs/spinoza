import { afterEach, describe, expect, it, vi } from 'vitest';
import { fetchHelmAccess, helmRefusalsOf } from '../../src/lib/helmAccess';
import { anySignal } from '../helpers';

afterEach(() => {
  vi.unstubAllGlobals();
});

function answers(body: unknown) {
  const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(body) });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

describe('asking the cluster what it would refuse a helm action', () => {
  it('asks about the release that is open', async () => {
    const fetchMock = answers({ refused: [] });

    await fetchHelmAccess('demo', 'podinfo');

    expect(fetchMock).toHaveBeenCalledWith('/api/helm/access?namespace=demo&name=podinfo', {
      signal: anySignal(),
    });
  });

  // Installing is asking about a release that is not there yet, so there is no
  // name to ask about.
  it('asks about the namespace alone when there is no release yet', async () => {
    const fetchMock = answers({ refused: [] });

    await fetchHelmAccess('demo', '');

    expect(fetchMock).toHaveBeenCalledWith('/api/helm/access?namespace=demo', {
      signal: anySignal(),
    });
  });

  it('reads the refusals and their reasons', async () => {
    answers({
      refused: [
        { capability: 'upgrade', reason: 'no creating secrets in demo' },
        { capability: 'uninstall', reason: 'no deleting secrets in demo' },
      ],
    });

    const access = await fetchHelmAccess('demo', 'podinfo');

    expect(helmRefusalsOf(access)).toEqual({
      upgrade: 'no creating secrets in demo',
      uninstall: 'no deleting secrets in demo',
    });
  });

  it('reads an empty answer as nothing refused', async () => {
    answers({});

    const access = await fetchHelmAccess('demo', 'podinfo');

    expect(helmRefusalsOf(access)).toEqual({});
  });

  it('surfaces the server message', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 400,
        json: () => Promise.resolve({ message: 'namespace is required' }),
      }),
    );

    await expect(fetchHelmAccess('', 'podinfo')).rejects.toThrow('namespace is required');
  });

  it('falls back to a status message', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 503,
        json: () => Promise.reject(new Error('not json')),
      }),
    );

    await expect(fetchHelmAccess('demo', 'podinfo')).rejects.toThrow(
      'helm access request failed with status 503',
    );
  });
});

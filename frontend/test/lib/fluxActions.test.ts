import { afterEach, describe, expect, it, vi } from 'vitest';
import { groupOf, isFluxObject, runFluxAction } from '../../src/lib/fluxActions';
import type { ObjectRef } from '../../src/lib/types';

const ref: ObjectRef = {
  group: 'kustomize.toolkit.fluxcd.io',
  version: 'v1',
  resource: 'kustomizations',
  namespace: 'flux-system',
  name: 'apps',
};

describe('fluxActions', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('extracts the group from an apiVersion', () => {
    expect(groupOf('kustomize.toolkit.fluxcd.io/v1')).toBe('kustomize.toolkit.fluxcd.io');
    expect(groupOf('v1')).toBe('');
  });

  it('recognises flux toolkit groups', () => {
    expect(isFluxObject('kustomize.toolkit.fluxcd.io/v1')).toBe(true);
    expect(isFluxObject('helm.toolkit.fluxcd.io/v2')).toBe(true);
    expect(isFluxObject('source.toolkit.fluxcd.io/v1')).toBe(true);
    expect(isFluxObject('notification.toolkit.fluxcd.io/v1beta3')).toBe(true);
    expect(isFluxObject('image.toolkit.fluxcd.io/v1beta2')).toBe(true);
  });

  it('rejects everything else', () => {
    expect(isFluxObject('apps/v1')).toBe(false);
    expect(isFluxObject('v1')).toBe(false);
    expect(isFluxObject('cilium.io/v2')).toBe(false);
    expect(isFluxObject('toolkit.fluxcd.io/v1')).toBe(false);
  });

  it('posts the action to the endpoint', async () => {
    const mock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({}) });
    vi.stubGlobal('fetch', mock);

    await expect(runFluxAction(ref, 'reconcile')).resolves.toBeUndefined();
    expect(mock).toHaveBeenCalledWith(
      '/api/flux/action?group=kustomize.toolkit.fluxcd.io&version=v1&resource=kustomizations&namespace=flux-system&name=apps&action=reconcile',
      { method: 'POST' },
    );
  });

  it('surfaces the server message on failure', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 404,
        json: () => Promise.resolve({ message: 'kustomizations "apps" not found' }),
      }),
    );

    await expect(runFluxAction(ref, 'suspend')).rejects.toThrow('kustomizations "apps" not found');
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

    await expect(runFluxAction(ref, 'resume')).rejects.toThrow('resume failed with status 500');
  });
});

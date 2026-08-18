import { afterEach, describe, expect, it, vi } from 'vitest';
import { isArgoApplication, runArgoAction } from '../../src/lib/argoActions';
import type { ObjectRef } from '../../src/lib/types';
import { anySignal } from '../helpers';

const ref: ObjectRef = {
  group: 'argoproj.io',
  version: 'v1alpha1',
  resource: 'applications',
  namespace: 'argocd',
  name: 'podinfo',
};

const query =
  'group=argoproj.io&version=v1alpha1&resource=applications&namespace=argocd&name=podinfo';

describe('which objects argo can act on', () => {
  it('takes applications', () => {
    expect(isArgoApplication('argoproj.io/v1alpha1', 'Application')).toBe(true);
  });

  it('leaves the other argo kinds alone', () => {
    expect(isArgoApplication('argoproj.io/v1alpha1', 'ApplicationSet')).toBe(false);
    expect(isArgoApplication('argoproj.io/v1alpha1', 'AppProject')).toBe(false);
  });

  it('rejects everything outside the group', () => {
    expect(isArgoApplication('apps/v1', 'Application')).toBe(false);
    expect(isArgoApplication('v1', 'Application')).toBe(false);
    expect(isArgoApplication('kustomize.toolkit.fluxcd.io/v1', 'Kustomization')).toBe(false);
  });
});

describe('asking argo cd to act', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('posts the action and returns what came back', async () => {
    const mock = vi
      .fn()
      .mockResolvedValue({ ok: true, json: () => Promise.resolve({ action: 'sync' }) });
    vi.stubGlobal('fetch', mock);

    await expect(runArgoAction(ref, 'sync')).resolves.toEqual({ action: 'sync' });
    expect(mock).toHaveBeenCalledWith(`/api/argocd/action?${query}&action=sync`, {
      method: 'POST',
      signal: anySignal(),
    });
  });

  it('carries the typed name when the cluster is protected', async () => {
    const mock = vi
      .fn()
      .mockResolvedValue({ ok: true, json: () => Promise.resolve({ action: 'sync' }) });
    vi.stubGlobal('fetch', mock);

    await runArgoAction(ref, 'sync', 'podinfo');

    expect(mock).toHaveBeenCalledWith(`/api/argocd/action?${query}&action=sync&confirm=podinfo`, {
      method: 'POST',
      signal: anySignal(),
    });
  });

  it('surfaces the server message on failure', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 412,
        json: () => Promise.resolve({ message: 'this cluster is protected' }),
      }),
    );

    await expect(runArgoAction(ref, 'sync')).rejects.toThrow('this cluster is protected');
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

    await expect(runArgoAction(ref, 'refresh')).rejects.toThrow('refresh failed with status 500');
  });
});

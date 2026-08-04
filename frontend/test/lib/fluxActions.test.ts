import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  groupOf,
  isFluxObject,
  isSettled,
  pollReconcile,
  RECONCILE_POLL_MS,
  RECONCILE_TIMEOUT_MS,
  readyCondition,
  reconcileProgress,
  runFluxAction,
} from '../../src/lib/fluxActions';
import type { ObjectDetail, ObjectRef } from '../../src/lib/types';
import { anySignal } from '../helpers';

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

  it('posts the action to the endpoint and returns the token', async () => {
    const result = { action: 'reconcile', requestedAt: '2026-07-28T12:00:00.5Z' };
    const mock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(result) });
    vi.stubGlobal('fetch', mock);

    await expect(runFluxAction(ref, 'reconcile')).resolves.toEqual(result);
    expect(mock).toHaveBeenCalledWith(
      '/api/flux/action?group=kustomize.toolkit.fluxcd.io&version=v1&resource=kustomizations&namespace=flux-system&name=apps&action=reconcile',
      { method: 'POST', signal: anySignal() },
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

const TOKEN = '2026-07-28T12:00:00.5Z';

function detail(overrides: Partial<ObjectDetail> = {}): ObjectDetail {
  return {
    apiVersion: 'kustomize.toolkit.fluxcd.io/v1',
    kind: 'Kustomization',
    name: 'apps',
    namespace: 'flux-system',
    uid: 'uid-1',
    createdAt: '2026-07-01T00:00:00Z',
    yaml: '',
    ...overrides,
  };
}

describe('reconcileProgress', () => {
  it('waits while the controller has not picked the request up', () => {
    const progress = reconcileProgress(
      detail({ flux: { suspended: false, handledAt: 'older' } }),
      TOKEN,
    );

    expect(progress.state).toBe('requested');
    expect(progress.message).toBe('Reconciliation requested…');
  });

  it('reports running once handled but with no Ready condition yet', () => {
    const progress = reconcileProgress(
      detail({ flux: { suspended: false, handledAt: TOKEN } }),
      TOKEN,
    );

    expect(progress.state).toBe('running');
  });

  it('reports success from a true Ready condition', () => {
    const progress = reconcileProgress(
      detail({
        flux: { suspended: false, handledAt: TOKEN },
        conditions: [{ type: 'Ready', status: 'True', message: 'Applied revision main@sha1:abc' }],
      }),
      TOKEN,
    );

    expect(progress.state).toBe('succeeded');
    expect(progress.message).toBe('Reconciliation succeeded: Applied revision main@sha1:abc');
  });

  it('reports failure with the controller message', () => {
    const progress = reconcileProgress(
      detail({
        flux: { suspended: false, handledAt: TOKEN },
        conditions: [{ type: 'Ready', status: 'False', message: 'kustomize build failed' }],
      }),
      TOKEN,
    );

    expect(progress.state).toBe('failed');
    expect(progress.message).toBe('Reconciliation failed: kustomize build failed');
  });

  it('keeps waiting while Ready is unknown', () => {
    const progress = reconcileProgress(
      detail({
        flux: { suspended: false, handledAt: TOKEN },
        conditions: [{ type: 'Ready', status: 'Unknown', message: 'reconciliation in progress' }],
      }),
      TOKEN,
    );

    expect(progress.state).toBe('running');
    expect(progress.message).toBe('Reconciliation running: reconciliation in progress');
  });

  it('drops the colon when the condition carries no message', () => {
    const progress = reconcileProgress(
      detail({
        flux: { suspended: false, handledAt: TOKEN },
        conditions: [{ type: 'Ready', status: 'True' }],
      }),
      TOKEN,
    );

    expect(progress.message).toBe('Reconciliation succeeded.');
  });

  it('finds the Ready condition among others', () => {
    const found = readyCondition(
      detail({
        conditions: [
          { type: 'Reconciling', status: 'True' },
          { type: 'Ready', status: 'False' },
        ],
      }),
    );

    expect(found?.status).toBe('False');
  });

  it('returns nothing when there is no Ready condition', () => {
    expect(readyCondition(detail())).toBeNull();
    expect(
      readyCondition(detail({ conditions: [{ type: 'Stalled', status: 'True' }] })),
    ).toBeNull();
  });

  it('settles only on a terminal state', () => {
    expect(isSettled('succeeded')).toBe(true);
    expect(isSettled('failed')).toBe(true);
    expect(isSettled('running')).toBe(false);
    expect(isSettled('requested')).toBe(false);
  });
});

describe('pollReconcile', () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  function wire(one: ObjectDetail): Record<string, unknown> {
    const { flux, ...rest } = one;
    return { ...rest, suspended: flux?.suspended, handledAt: flux?.handledAt };
  }

  function stubObject(...responses: ObjectDetail[]) {
    let call = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(() => {
        const body = wire(responses[Math.min(call, responses.length - 1)]);
        call += 1;
        return Promise.resolve({ ok: true, json: () => Promise.resolve(body) });
      }),
    );
  }

  it('reports each step until the reconcile settles', async () => {
    stubObject(
      detail({ flux: { suspended: false, handledAt: 'older' } }),
      detail({
        flux: { suspended: false, handledAt: TOKEN },
        conditions: [{ type: 'Ready', status: 'Unknown' }],
      }),
      detail({
        flux: { suspended: false, handledAt: TOKEN },
        conditions: [{ type: 'Ready', status: 'True' }],
      }),
    );
    const seen: string[] = [];

    await pollReconcile(ref, TOKEN, (progress) => {
      seen.push(progress.state);
      return true;
    });

    expect(seen).toEqual(['requested', 'running', 'succeeded']);
  });

  it('stops when the caller loses interest', async () => {
    stubObject(detail({ flux: { suspended: false, handledAt: 'older' } }));
    const seen: string[] = [];

    await pollReconcile(ref, TOKEN, (progress) => {
      seen.push(progress.state);
      return false;
    });

    expect(seen).toEqual(['requested']);
  });

  it('rides out a failed fetch', async () => {
    let call = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(() => {
        call += 1;
        if (call === 1) {
          return Promise.reject(new Error('offline'));
        }
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve(
              wire(
                detail({
                  flux: { suspended: false, handledAt: TOKEN },
                  conditions: [{ type: 'Ready', status: 'True' }],
                }),
              ),
            ),
        });
      }),
    );
    const seen: string[] = [];

    await pollReconcile(ref, TOKEN, (progress) => {
      seen.push(progress.state);
      return true;
    });

    expect(seen).toEqual(['succeeded']);
  });

  it('gives up after the timeout without claiming success', async () => {
    vi.useFakeTimers();
    stubObject(detail({ flux: { suspended: false, handledAt: 'older' } }));
    const seen: string[] = [];

    const done = pollReconcile(ref, TOKEN, (progress) => {
      seen.push(progress.message);
      return true;
    });
    await vi.advanceTimersByTimeAsync(RECONCILE_TIMEOUT_MS + RECONCILE_POLL_MS);
    await done;

    expect(seen[seen.length - 1]).toBe('Reconciliation is still running.');
  });
});

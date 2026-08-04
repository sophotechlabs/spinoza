import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  canRestart,
  canScale,
  countBy,
  hasActions,
  isCordoned,
  isNode,
  replicasOf,
  runAction,
} from '../../src/lib/objectActions';
import type { ActionResult, ObjectDetail, ObjectRef } from '../../src/lib/types';
import { anySignal } from '../helpers';

function ref(group: string, resource: string): ObjectRef {
  return { group, version: 'v1', resource, namespace: 'shop', name: 'web' };
}

function detail(overrides: Partial<ObjectDetail>): ObjectDetail {
  return {
    apiVersion: 'apps/v1',
    kind: 'Deployment',
    name: 'web',
    namespace: 'shop',
    uid: 'uid',
    createdAt: '2026-07-31T10:00:00Z',
    yaml: '',
    ...overrides,
  };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('which actions a resource takes', () => {
  it('scales the workloads that have a scale subresource', () => {
    expect(canScale(ref('apps', 'deployments'))).toBe(true);
    expect(canScale(ref('apps', 'statefulsets'))).toBe(true);
    expect(canScale(ref('apps', 'replicasets'))).toBe(true);
    expect(canScale(ref('', 'replicationcontrollers'))).toBe(true);
    expect(canScale(ref('apps', 'daemonsets'))).toBe(false);
    expect(canScale(ref('', 'pods'))).toBe(false);
  });

  it('restarts the workloads that own a pod template', () => {
    expect(canRestart(ref('apps', 'deployments'))).toBe(true);
    expect(canRestart(ref('apps', 'statefulsets'))).toBe(true);
    expect(canRestart(ref('apps', 'daemonsets'))).toBe(true);
    expect(canRestart(ref('apps', 'replicasets'))).toBe(false);
  });

  it('recognises a node', () => {
    expect(isNode(ref('', 'nodes'))).toBe(true);
    expect(isNode(ref('metrics.k8s.io', 'nodes'))).toBe(false);
  });

  it('shows the panel only for a resource with actions', () => {
    expect(hasActions(ref('apps', 'deployments'))).toBe(true);
    expect(hasActions(ref('apps', 'daemonsets'))).toBe(true);
    expect(hasActions(ref('', 'nodes'))).toBe(true);
    expect(hasActions(ref('', 'configmaps'))).toBe(false);
  });
});

describe('runAction', () => {
  it('posts the action with the replica count', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ action: 'scale', message: 'ok' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await runAction(ref('apps', 'deployments'), 'scale', { replicas: 3 });

    const call = fetchMock.mock.calls[0];
    expect(String(call[0])).toContain('action=scale');
    expect(String(call[0])).toContain('replicas=3');
    expect(String(call[0])).toContain('resource=deployments');
    expect(call[1]).toEqual({
      method: 'POST',
      signal: anySignal(),
    });
  });

  it('sends replicas=0 rather than dropping it', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ action: 'scale', message: 'ok' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await runAction(ref('apps', 'deployments'), 'scale', { replicas: 0 });

    expect(String(fetchMock.mock.calls[0][0])).toContain('replicas=0');
  });

  it('leaves force and dryRun off unless asked', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ action: 'drain', message: 'ok' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await runAction(ref('', 'nodes'), 'drain');

    const url = String(fetchMock.mock.calls[0][0]);
    expect(url).not.toContain('force');
    expect(url).not.toContain('dryRun');
  });

  it('asks for a plan and for a forced drain', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ action: 'drain', message: 'ok' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await runAction(ref('', 'nodes'), 'drain', { dryRun: true, force: true });

    const url = String(fetchMock.mock.calls[0][0]);
    expect(url).toContain('dryRun=true');
    expect(url).toContain('force=true');
  });

  it('surfaces the server message', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 400,
        json: () => Promise.resolve({ message: '2 pods cannot be evicted safely' }),
      }),
    );

    await expect(runAction(ref('', 'nodes'), 'drain')).rejects.toThrow(
      '2 pods cannot be evicted safely',
    );
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

    await expect(runAction(ref('apps', 'deployments'), 'restart')).rejects.toThrow(
      'restart failed with status 500',
    );
  });
});

describe('reading the detail', () => {
  it('reads the replica count, defaulting to zero', () => {
    expect(replicasOf(detail({ workload: { replicas: 4 } }))).toBe(4);
    expect(replicasOf(detail({ workload: { replicas: 0 } }))).toBe(0);
    expect(replicasOf(detail({}))).toBe(0);
    expect(replicasOf(null)).toBe(0);
  });

  it('knows a cordoned node from a schedulable one', () => {
    expect(isCordoned(detail({ node: { schedulable: false } }))).toBe(true);
    expect(isCordoned(detail({ node: { schedulable: true } }))).toBe(false);
    expect(isCordoned(detail({}))).toBe(false);
    expect(isCordoned(null)).toBe(false);
  });

  it('counts pods by outcome', () => {
    const result: ActionResult = {
      action: 'drain',
      message: '',
      pods: [
        { namespace: 'a', name: 'one', outcome: 'evict' },
        { namespace: 'a', name: 'two', outcome: 'blocked' },
        { namespace: 'a', name: 'three', outcome: 'blocked' },
      ],
    };
    expect(countBy(result, 'blocked')).toBe(2);
    expect(countBy(result, 'evict')).toBe(1);
    expect(countBy({ action: 'drain', message: '' }, 'blocked')).toBe(0);
  });
});

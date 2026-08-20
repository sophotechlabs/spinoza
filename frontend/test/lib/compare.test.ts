import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  changedSections,
  differingLines,
  fetchComparison,
  fetchKindComparison,
} from '../../src/lib/compare';
import type { ObjectRef } from '../../src/lib/types';
import { anySignal } from '../helpers';

const ref: ObjectRef = {
  group: 'apps',
  version: 'v1',
  resource: 'deployments',
  namespace: 'prod',
  name: 'web',
};

const target = {
  kubeconfig: '/home/arch/.kube/config',
  name: 'gke-prod',
  namespace: 'prod',
  object: 'web',
};

const answer = {
  left: 'spec:\n  replicas: 3\n',
  right: 'spec:\n  replicas: 5\n',
  leftContext: 'staging',
  rightContext: 'gke-prod',
  identical: false,
};

function stub(body: unknown, ok = true) {
  const mock = vi
    .fn()
    .mockResolvedValue({ ok, status: ok ? 200 : 500, json: () => Promise.resolve(body) });
  vi.stubGlobal('fetch', mock);
  return mock;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

// how much of the object moved

describe('counting what differs', () => {
  it('counts nothing for two identical objects', () => {
    expect(differingLines('a\nb\n', 'a\nb\n')).toBe(0);
  });

  it('counts both sides of a changed line', () => {
    expect(differingLines('spec:\n  replicas: 3\n', 'spec:\n  replicas: 5\n')).toBe(2);
  });

  it('counts a line that only one side has', () => {
    expect(differingLines('a\n', 'a\nb\n')).toBe(1);
  });
});

describe('naming the sections that moved', () => {
  it('names the top-level key a change sits under', () => {
    expect(changedSections('spec:\n  replicas: 3\n', 'spec:\n  replicas: 5\n')).toEqual(['spec']);
  });

  it('names each one, in order', () => {
    const left = 'metadata:\n  name: a\nspec:\n  replicas: 3\n';
    const right = 'metadata:\n  name: b\nspec:\n  replicas: 5\n';

    expect(changedSections(left, right)).toEqual(['metadata', 'spec']);
  });

  it('names none when the two agree', () => {
    expect(changedSections('spec:\n  replicas: 3\n', 'spec:\n  replicas: 3\n')).toEqual([]);
  });
});

// what it asks the server for

describe('asking for a comparison', () => {
  it('names the context and its kubeconfig', async () => {
    const mock = stub(answer);

    await fetchComparison(ref, target, false);

    const url = mock.mock.calls[0][0] as string;
    expect(url).toContain('against=gke-prod');
    expect(url).toContain('againstKubeconfig=%2Fhome%2Farch%2F.kube%2Fconfig');
    expect(url).not.toContain('raw=');
  });

  it('leaves the far side out when it matches this one', async () => {
    const mock = stub(answer);

    await fetchComparison(ref, target, false);

    const url = mock.mock.calls[0][0] as string;
    expect(url).not.toContain('againstNamespace');
    expect(url).not.toContain('againstName=');
  });

  it('sends a namespace of its own when it differs', async () => {
    const mock = stub(answer);

    await fetchComparison(ref, { ...target, namespace: 'staging' }, false);

    expect(mock.mock.calls[0][0]).toContain('againstNamespace=staging');
  });

  it('asks for everything when raw is wanted', async () => {
    const mock = stub(answer);

    await fetchComparison(ref, target, true);

    expect(mock.mock.calls[0][0]).toContain('raw=true');
    expect(mock.mock.calls[0][1]).toEqual({ signal: anySignal() });
  });

  it('returns what came back', async () => {
    stub(answer);

    await expect(fetchComparison(ref, target, false)).resolves.toEqual({
      ...answer,
      missing: undefined,
    });
  });

  it('surfaces the reason it failed', async () => {
    stub({ message: 'that context has no such object' }, false);

    await expect(fetchComparison(ref, target, false)).rejects.toThrow('no such object');
  });
});

describe('a far side with a different name', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('sends the name when it differs from this one', async () => {
    const mock = stub(answer);

    await fetchComparison(ref, { ...target, object: 'api' }, false);

    expect(mock.mock.calls[0][0]).toContain('againstName=api');
  });
});

describe('comparing a whole kind', () => {
  const kind = {
    group: 'apps',
    version: 'v1',
    resource: 'deployments',
    kind: 'Deployment',
    namespaced: true,
    category: 'Workloads',
  };

  it('names the kind, the namespace and the context', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          resource: 'deployments',
          leftContext: 'p-mk1',
          rightContext: 'p-mk2',
          objects: [{ name: 'web', verdict: 'differs', lines: 3 }],
          same: 0,
          differs: 1,
          onlyHere: 0,
          onlyThere: 0,
        }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const got = await fetchKindComparison(kind, 'flux-system', {
      kubeconfig: '/work.yaml',
      name: 'p-mk2',
      namespace: 'flux-system',
      object: '',
    });

    const url = String(fetchMock.mock.calls[0][0]);
    expect(url).toContain('resource=deployments');
    expect(url).toContain('namespace=flux-system');
    expect(url).toContain('against=p-mk2');
    expect(url).toContain('againstKubeconfig=%2Fwork.yaml');
    expect(url).not.toContain('againstNamespace');
    expect(got.objects[0].lines).toBe(3);
  });

  it('names the far namespace only when it is a different one', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          resource: 'deployments',
          leftContext: 'a',
          rightContext: 'b',
          objects: [],
          same: 0,
          differs: 0,
          onlyHere: 0,
          onlyThere: 0,
        }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await fetchKindComparison(kind, 'prod', {
      kubeconfig: '',
      name: 'b',
      namespace: 'staging',
      object: '',
    });

    expect(String(fetchMock.mock.calls[0][0])).toContain('againstNamespace=staging');
  });

  it('carries the failure the server reports', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 404,
        json: () =>
          Promise.resolve({ message: 'the server could not find the requested resource' }),
      }),
    );

    await expect(
      fetchKindComparison(kind, 'prod', {
        kubeconfig: '',
        name: 'b',
        namespace: 'prod',
        object: '',
      }),
    ).rejects.toThrow('could not find');
  });

  it('falls back to the status when the body says nothing', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue({ ok: false, status: 502, json: () => Promise.reject(new Error()) }),
    );

    await expect(
      fetchKindComparison(kind, 'prod', {
        kubeconfig: '',
        name: 'b',
        namespace: 'prod',
        object: '',
      }),
    ).rejects.toThrow('comparing the kind failed with status 502');
  });
});

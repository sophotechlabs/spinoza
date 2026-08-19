import { afterEach, describe, expect, it, vi } from 'vitest';
import { changedSections, differingLines, fetchComparison } from '../../src/lib/compare';
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

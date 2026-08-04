import { beforeEach, describe, expect, it } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import type { ObjectRef } from '../../src/lib/types';
import { refFromFlux, refFromNode, refFromRow, sameGvr, useRowForRef } from '../../src/lib/refs';
import { useResourcesStore } from '../../src/store/resources';
import { makeColumns, makeDescriptor, makeFluxResource, makeGraphNode, makeRow } from '../helpers';

describe('refs', () => {
  it('builds a ref from the active descriptor and a row', () => {
    const descriptor = makeDescriptor({ group: 'apps', version: 'v1', resource: 'deployments' });
    const row = makeRow({ name: 'web', namespace: 'flux-system' });

    expect(refFromRow(descriptor, row)).toEqual({
      group: 'apps',
      version: 'v1',
      resource: 'deployments',
      namespace: 'flux-system',
      name: 'web',
    });
  });

  it('returns null for a row without an active descriptor', () => {
    expect(refFromRow(null, makeRow({}))).toBeNull();
  });

  it('builds a ref from a graph node', () => {
    const node = makeGraphNode({
      group: 'helm.toolkit.fluxcd.io',
      version: 'v2',
      resource: 'helmreleases',
      namespace: 'apps',
      name: 'podinfo',
    });

    expect(refFromNode(node)).toEqual({
      group: 'helm.toolkit.fluxcd.io',
      version: 'v2',
      resource: 'helmreleases',
      namespace: 'apps',
      name: 'podinfo',
    });
  });

  it('returns null for a graph node with an unresolved resource', () => {
    expect(refFromNode(makeGraphNode({ resource: '' }))).toBeNull();
  });

  it('builds a ref from a flux resource', () => {
    const resource = makeFluxResource({ name: 'apps', namespace: 'flux-system' });

    expect(refFromFlux(resource)).toEqual({
      group: 'kustomize.toolkit.fluxcd.io',
      version: 'v1',
      resource: 'kustomizations',
      namespace: 'flux-system',
      name: 'apps',
    });
  });
});

describe('sameGvr', () => {
  const base = { group: 'apps', version: 'v1', resource: 'deployments' };

  it('accepts an identical triple', () => {
    expect(sameGvr(base, { ...base })).toBe(true);
  });

  it('rejects a different group', () => {
    expect(sameGvr(base, { ...base, group: '' })).toBe(false);
  });

  it('rejects a different version', () => {
    expect(sameGvr(base, { ...base, version: 'v1beta1' })).toBe(false);
  });

  it('rejects a different resource', () => {
    expect(sameGvr(base, { ...base, resource: 'statefulsets' })).toBe(false);
  });
});

describe('useRowForRef', () => {
  const descriptor = makeDescriptor({ group: '', version: 'v1', resource: 'pods', kind: 'Pod' });
  const ref: ObjectRef = {
    group: '',
    version: 'v1',
    resource: 'pods',
    namespace: 'prod',
    name: 'web-0',
  };

  function seed(): void {
    useResourcesStore
      .getState()
      .applySnapshot('main#1', makeColumns([]), true, [
        makeRow({ uid: 'a', name: 'web-0', namespace: 'prod' }),
        makeRow({ uid: 'b', name: 'web-0', namespace: 'staging' }),
      ]);
  }

  beforeEach(() => {
    useResourcesStore.setState({ subs: new Map(), errors: new Map() });
  });

  it('finds the row by namespace and name, not by uid', () => {
    seed();

    const { result } = renderHook(() => useRowForRef('main#1', descriptor, ref));

    expect(result.current?.uid).toBe('a');
  });

  it('re-binds when the object comes back under a new uid', () => {
    seed();
    const { result } = renderHook(() => useRowForRef('main#1', descriptor, ref));

    act(() => {
      useResourcesStore
        .getState()
        .applyDeltas('main#1', [{ type: 'deleted', subId: 'main#1', uid: 'a' }]);
      useResourcesStore.getState().applyDeltas('main#1', [
        {
          type: 'added',
          subId: 'main#1',
          row: makeRow({ uid: 'c', name: 'web-0', namespace: 'prod' }),
        },
      ]);
    });

    expect(result.current?.uid).toBe('c');
  });

  it('has nothing to resolve without a ref', () => {
    seed();

    const { result } = renderHook(() => useRowForRef('main#1', descriptor, null));

    expect(result.current).toBeNull();
  });

  it('has nothing to resolve without an active resource', () => {
    seed();

    const { result } = renderHook(() => useRowForRef('main#1', null, ref));

    expect(result.current).toBeNull();
  });

  it('refuses to match a name across a different resource', () => {
    seed();
    const other = makeDescriptor({ group: 'apps', version: 'v1', resource: 'deployments' });

    const { result } = renderHook(() => useRowForRef('main#1', other, ref));

    expect(result.current).toBeNull();
  });

  it('waits for the subscription it was given', () => {
    seed();

    const { result } = renderHook(() => useRowForRef('main#2', descriptor, ref));

    expect(result.current).toBeNull();
  });

  it('reports nothing while the object is gone', () => {
    seed();

    const { result } = renderHook(() =>
      useRowForRef('main#1', descriptor, { ...ref, name: 'web-9' }),
    );

    expect(result.current).toBeNull();
  });
});

import { describe, expect, it } from 'vitest';
import { refFromFlux, refFromNode, refFromRow } from '../../src/lib/refs';
import { makeDescriptor, makeFluxResource, makeGraphNode, makeRow } from '../helpers';

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

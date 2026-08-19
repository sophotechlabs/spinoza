import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import type { Route } from '../../src/lib/router';
import { VIEWS } from '../../src/lib/types';
import {
  EMPTY_ROUTE,
  decodeRoute,
  descriptorOf,
  documentTitle,
  encodeRoute,
  resourceKey,
  useRouter,
} from '../../src/lib/router';

const pods = { group: '', version: 'v1', resource: 'pods', kind: 'Pod' };
const deployments = { group: 'apps', version: 'v1', resource: 'deployments', kind: 'Deployment' };

function route(overrides: Partial<Route> = {}): Route {
  return { ...EMPTY_ROUTE, ...overrides };
}

function roundTrip(value: Route): Route {
  return decodeRoute(encodeRoute(value));
}

function goTo(hash: string): void {
  window.history.replaceState(null, '', hash === '' ? '/' : hash);
}

describe('encoding a route', () => {
  it('writes nothing for the empty route', () => {
    expect(encodeRoute(EMPTY_ROUTE)).toBe('');
  });

  it('leaves the view out when the resource already implies it', () => {
    expect(encodeRoute(route({ view: 'resources', resource: pods }))).toBe(
      '#version=v1&resource=pods&kind=Pod',
    );
  });

  it('leaves the view out when the overview is what an empty route means', () => {
    expect(encodeRoute(route({ view: 'cluster' }))).toBe('');
  });

  it('names the table when it is asked for without a resource', () => {
    expect(encodeRoute(route({ view: 'resources' }))).toBe('#view=resources');
  });

  it('names a view that is not the default', () => {
    expect(encodeRoute(route({ view: 'gitops' }))).toBe('#view=gitops');
  });

  it('carries the context it was taken against', () => {
    expect(encodeRoute(route({ context: 'kind-dev' }))).toBe('#context=kind-dev');
  });

  it('leaves the selection gvr out when it matches the table', () => {
    const hash = encodeRoute(
      route({
        resource: deployments,
        selection: {
          group: 'apps',
          version: 'v1',
          resource: 'deployments',
          namespace: 'prod',
          name: 'web',
        },
      }),
    );

    expect(hash).toContain('namespace=prod');
    expect(hash).toContain('name=web');
    expect(hash).not.toContain('selResource');
  });

  it('spells out a selection that came from another resource', () => {
    const hash = encodeRoute(
      route({
        resource: pods,
        selection: {
          group: 'helm.toolkit.fluxcd.io',
          version: 'v2',
          resource: 'helmreleases',
          namespace: 'apps',
          name: 'podinfo',
        },
      }),
    );

    expect(hash).toContain('selResource=helmreleases');
    expect(hash).toContain('selGroup=helm.toolkit.fluxcd.io');
    expect(hash).toContain('selVersion=v2');
  });
});

describe('decoding a route', () => {
  it('reads an empty hash as the starting point', () => {
    expect(decodeRoute('')).toEqual(EMPTY_ROUTE);
  });

  it('ignores a view it does not know', () => {
    expect(decodeRoute('#view=nonsense').view).toBe('cluster');
  });

  it('opens on the overview when the url names nothing', () => {
    expect(decodeRoute('').view).toBe('cluster');
  });

  it('opens on the table when the url names a resource', () => {
    expect(decodeRoute('#version=v1&resource=pods&kind=Pod').view).toBe('resources');
  });

  it('accepts every view the app has', () => {
    for (const view of VIEWS) {
      expect(decodeRoute(encodeRoute(route({ view }))).view).toBe(view);
    }
  });

  it('round-trips a resource in the core group', () => {
    expect(roundTrip(route({ resource: pods })).resource).toEqual(pods);
  });

  it('round-trips a table selection', () => {
    const selection = {
      group: 'apps',
      version: 'v1',
      resource: 'deployments',
      namespace: 'prod',
      name: 'web',
    };

    expect(roundTrip(route({ resource: deployments, selection })).selection).toEqual(selection);
  });

  it('round-trips a selection taken from another view', () => {
    const selection = {
      group: '',
      version: 'v1',
      resource: 'pods',
      namespace: 'kube-system',
      name: 'coredns-0',
    };

    expect(roundTrip(route({ resource: null, selection })).selection).toEqual(selection);
  });

  it('round-trips a cluster-scoped selection', () => {
    const nodes = { group: '', version: 'v1', resource: 'nodes', kind: 'Node' };
    const selection = { group: '', version: 'v1', resource: 'nodes', namespace: '', name: 'w-1' };

    expect(roundTrip(route({ resource: nodes, selection })).selection).toEqual(selection);
  });

  it('drops a name it cannot attach to any resource', () => {
    expect(decodeRoute('#name=orphan').selection).toBeNull();
  });

  it('drops a resource that has no name of its own', () => {
    expect(decodeRoute('#group=apps&version=v1').resource).toBeNull();
  });

  it('round-trips the context', () => {
    expect(roundTrip(route({ context: 'p-mk1' })).context).toBe('p-mk1');
  });
});

describe('the release in the address bar', () => {
  it('writes the release next to whatever else the route holds', () => {
    const hash = encodeRoute(
      route({ view: 'helm', release: { namespace: 'demo', name: 'podinfo' } }),
    );

    expect(hash).toContain('view=helm');
    expect(hash).toContain('release=podinfo');
    expect(hash).toContain('releaseNs=demo');
  });

  it('round-trips a release', () => {
    const release = { namespace: 'demo', name: 'podinfo' };

    expect(roundTrip(route({ view: 'helm', release })).release).toEqual(release);
  });

  it('round-trips a release with no namespace of its own', () => {
    const release = { namespace: '', name: 'podinfo' };

    expect(roundTrip(route({ view: 'helm', release })).release).toEqual(release);
  });

  it('reads an old url without release params as no release', () => {
    expect(decodeRoute('#view=helm').release).toBeNull();
  });

  it('drops a release namespace that names no release', () => {
    expect(decodeRoute('#releaseNs=demo').release).toBeNull();
  });

  it('keeps the release apart from an object selection', () => {
    const decoded = roundTrip(
      route({
        resource: pods,
        selection: { group: '', version: 'v1', resource: 'pods', namespace: 'p', name: 'web-0' },
        release: { namespace: 'demo', name: 'podinfo' },
      }),
    );

    expect(decoded.selection?.name).toBe('web-0');
    expect(decoded.release).toEqual({ namespace: 'demo', name: 'podinfo' });
  });
});

describe('resourceKey', () => {
  it('is empty without a resource', () => {
    expect(resourceKey(null)).toBe('');
  });

  it('separates two resources that differ only by group', () => {
    expect(resourceKey(pods)).not.toBe(resourceKey({ ...pods, group: 'metrics.k8s.io' }));
  });
});

describe('descriptorOf', () => {
  it('keeps the identity the url carried', () => {
    expect(descriptorOf(deployments)).toMatchObject(deployments);
  });
});

describe('the document title', () => {
  it('is the bare app name with nothing chosen', () => {
    expect(documentTitle(EMPTY_ROUTE)).toBe('Spinoza');
  });

  it('names the resource and the cluster', () => {
    expect(documentTitle(route({ view: 'resources', context: 'kind-dev', resource: pods }))).toBe(
      'pods kind-dev - Spinoza',
    );
  });

  it('leads with the selected object', () => {
    const selection = { group: '', version: 'v1', resource: 'pods', namespace: 'p', name: 'web-0' };

    expect(
      documentTitle(route({ view: 'resources', context: 'kind-dev', resource: pods, selection })),
    ).toBe('web-0 pods kind-dev - Spinoza');
  });

  it('names the view instead of the resource outside the table', () => {
    expect(documentTitle(route({ view: 'gitops', resource: pods }))).toBe('gitops - Spinoza');
  });
});

describe('useRouter', () => {
  beforeEach(() => {
    goTo('');
  });

  afterEach(() => {
    goTo('');
  });

  it('starts from the hash the page was opened with', () => {
    goTo('#view=flux-list&context=p-mk1');

    const { result } = renderHook(() => useRouter());

    expect(result.current.route.view).toBe('flux-list');
    expect(result.current.route.context).toBe('p-mk1');
  });

  it('writes a navigation into the address bar and the history', () => {
    const { result } = renderHook(() => useRouter());
    const before = window.history.length;

    act(() => {
      result.current.navigate(route({ view: 'resources', resource: pods }));
    });

    expect(window.location.hash).toBe('#version=v1&resource=pods&kind=Pod');
    expect(result.current.route.resource).toEqual(pods);
    expect(window.history.length).toBeGreaterThan(before);
  });

  it('replaces without growing the history', () => {
    const { result } = renderHook(() => useRouter());
    const before = window.history.length;

    act(() => {
      result.current.replace(route({ context: 'p-mk1' }));
    });

    expect(window.location.hash).toBe('#context=p-mk1');
    expect(window.history.length).toBe(before);
  });

  it('clears the hash when it goes back to the starting point', () => {
    goTo('#view=gitops');
    const { result } = renderHook(() => useRouter());

    act(() => {
      result.current.navigate(EMPTY_ROUTE);
    });

    expect(window.location.hash).toBe('');
  });

  it('does not push a second entry for the same route', () => {
    goTo('#view=gitops');
    const { result } = renderHook(() => useRouter());
    const before = window.history.length;

    act(() => {
      result.current.navigate(route({ view: 'gitops' }));
    });

    expect(window.history.length).toBe(before);
  });

  it('does not replace with a hash that is already there', () => {
    goTo('#view=gitops');
    const { result } = renderHook(() => useRouter());

    act(() => {
      result.current.replace(route({ view: 'gitops' }));
    });

    expect(window.location.hash).toBe('#view=gitops');
  });

  it('follows the back button', () => {
    const { result } = renderHook(() => useRouter());
    act(() => {
      result.current.navigate(route({ view: 'resources', resource: pods }));
    });

    act(() => {
      goTo('');
      window.dispatchEvent(new PopStateEvent('popstate'));
    });

    expect(result.current.route.resource).toBeNull();
  });

  it('follows a hash typed straight into the address bar', () => {
    const { result } = renderHook(() => useRouter());

    act(() => {
      goTo('#view=flux-roles');
      window.dispatchEvent(new HashChangeEvent('hashchange'));
    });

    expect(result.current.route.view).toBe('flux-roles');
  });

  it('stops listening once it goes away', () => {
    const { result, unmount } = renderHook(() => useRouter());
    unmount();

    act(() => {
      goTo('#view=gitops');
      window.dispatchEvent(new PopStateEvent('popstate'));
    });

    expect(result.current.route.view).toBe('cluster');
  });

  it('restores the argo applications view from the address bar', () => {
    expect(decodeRoute('view=argo-apps').view).toBe('argo-apps');
    expect(VIEWS).toContain('argo-apps');
  });
});

describe('the title of a docked release', () => {
  const release = { namespace: 'cert-manager', name: 'cert-manager' };

  it('names the release when nothing else is selected', () => {
    expect(documentTitle(route({ view: 'helm', context: 'p-mk1', release }))).toBe(
      'cert-manager helm p-mk1 - Spinoza',
    );
  });

  it('lets the selected object win', () => {
    const selection = { group: '', version: 'v1', resource: 'pods', namespace: 'p', name: 'web-0' };

    expect(documentTitle(route({ view: 'resources', context: 'p-mk1', release, selection }))).toBe(
      'web-0 resources p-mk1 - Spinoza',
    );
  });
});

import { useCallback, useEffect, useState } from 'react';
import type { ObjectRef, ResourceDescriptor, View } from './types';

export const VIEWS: View[] = ['resources', 'gitops', 'flux-list', 'flux-overview', 'flux-roles'];

export interface RouteResource {
  group: string;
  version: string;
  resource: string;
  kind: string;
}

export interface Route {
  context: string;
  view: View;
  resource: RouteResource | null;
  selection: ObjectRef | null;
}

export const EMPTY_ROUTE: Route = {
  context: '',
  view: 'resources',
  resource: null,
  selection: null,
};

export function descriptorOf(resource: RouteResource): ResourceDescriptor {
  return {
    group: resource.group,
    version: resource.version,
    resource: resource.resource,
    kind: resource.kind,
    namespaced: false,
    category: '',
  };
}

function gvrKey(gvr: { group: string; version: string; resource: string }): string {
  return `${gvr.group}/${gvr.version}/${gvr.resource}`;
}

export function resourceKey(resource: RouteResource | null): string {
  if (resource === null) {
    return '';
  }
  return gvrKey(resource);
}

function isView(value: string): value is View {
  return (VIEWS as string[]).includes(value);
}

function put(params: URLSearchParams, key: string, value: string): void {
  if (value === '') {
    return;
  }
  params.set(key, value);
}

export function encodeRoute(route: Route): string {
  const params = new URLSearchParams();
  put(params, 'context', route.context);
  if (route.view !== 'resources') {
    params.set('view', route.view);
  }
  if (route.resource !== null) {
    put(params, 'group', route.resource.group);
    put(params, 'version', route.resource.version);
    put(params, 'resource', route.resource.resource);
    put(params, 'kind', route.resource.kind);
  }
  if (route.selection !== null) {
    put(params, 'namespace', route.selection.namespace);
    params.set('name', route.selection.name);
    if (gvrKey(route.selection) !== resourceKey(route.resource)) {
      put(params, 'selGroup', route.selection.group);
      put(params, 'selVersion', route.selection.version);
      put(params, 'selResource', route.selection.resource);
    }
  }
  const query = params.toString();
  if (query === '') {
    return '';
  }
  return `#${query}`;
}

function readResource(params: URLSearchParams): RouteResource | null {
  const resource = params.get('resource') ?? '';
  if (resource === '') {
    return null;
  }
  return {
    group: params.get('group') ?? '',
    version: params.get('version') ?? '',
    resource,
    kind: params.get('kind') ?? '',
  };
}

function readSelection(params: URLSearchParams, resource: RouteResource | null): ObjectRef | null {
  const name = params.get('name') ?? '';
  if (name === '') {
    return null;
  }
  const own = params.get('selResource') ?? '';
  if (own !== '') {
    return {
      group: params.get('selGroup') ?? '',
      version: params.get('selVersion') ?? '',
      resource: own,
      namespace: params.get('namespace') ?? '',
      name,
    };
  }
  if (resource === null) {
    return null;
  }
  return {
    group: resource.group,
    version: resource.version,
    resource: resource.resource,
    namespace: params.get('namespace') ?? '',
    name,
  };
}

export function decodeRoute(hash: string): Route {
  const params = new URLSearchParams(hash.replace(/^#/, ''));
  let view: View = 'resources';
  const named = params.get('view') ?? '';
  if (isView(named)) {
    view = named;
  }
  const resource = readResource(params);
  return {
    context: params.get('context') ?? '',
    view,
    resource,
    selection: readSelection(params, resource),
  };
}

export function documentTitle(route: Route): string {
  const parts: string[] = [];
  if (route.selection !== null) {
    parts.push(route.selection.name);
  }
  if (route.view !== 'resources') {
    parts.push(route.view);
  }
  if (route.view === 'resources' && route.resource !== null) {
    parts.push(route.resource.resource);
  }
  if (route.context !== '') {
    parts.push(route.context);
  }
  if (parts.length === 0) {
    return 'Spinoza';
  }
  return `${parts.join(' · ')} — Spinoza`;
}

export interface Router {
  route: Route;
  navigate: (next: Route) => void;
  replace: (next: Route) => void;
}

function currentHash(): string {
  return window.location.hash;
}

function urlFor(hash: string): string {
  if (hash === '') {
    return window.location.pathname + window.location.search;
  }
  return hash;
}

export function useRouter(): Router {
  const [route, setRoute] = useState<Route>(() => decodeRoute(currentHash()));

  useEffect(() => {
    function sync() {
      setRoute(decodeRoute(currentHash()));
    }
    window.addEventListener('popstate', sync);
    window.addEventListener('hashchange', sync);
    return () => {
      window.removeEventListener('popstate', sync);
      window.removeEventListener('hashchange', sync);
    };
  }, []);

  const navigate = useCallback((next: Route) => {
    setRoute(next);
    const hash = encodeRoute(next);
    if (hash === currentHash()) {
      return;
    }
    window.history.pushState(null, '', urlFor(hash));
  }, []);

  const replace = useCallback((next: Route) => {
    setRoute(next);
    const hash = encodeRoute(next);
    if (hash === currentHash()) {
      return;
    }
    window.history.replaceState(null, '', urlFor(hash));
  }, []);

  return { route, navigate, replace };
}

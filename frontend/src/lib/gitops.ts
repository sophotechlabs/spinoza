import type { Category, ResourceDescriptor, View } from './types';

const FLUX_SUFFIX = '.toolkit.fluxcd.io';

const ARGO_GROUP = 'argoproj.io';

type GitopsEngine = 'flux' | 'argo';

export const CLUSTER_ABSENCE = {
  flux: 'Flux is not found in this cluster',
  argo: 'Argo CD is not found in this cluster',
  traffic: 'Traffic is not found in this cluster',
};

const ENGINE_BY_VIEW: Partial<Record<View, GitopsEngine>> = {
  'flux-roles': 'flux',
  gitops: 'flux',
  'flux-list': 'flux',
  'argo-apps': 'argo',
  'argo-graph': 'argo',
  'argo-list': 'argo',
};

function descriptors(categories: Category[]): ResourceDescriptor[] {
  return categories.flatMap((category) => category.resources);
}

export function fluxInstalled(categories: Category[]): boolean {
  return descriptors(categories).some((descriptor) => descriptor.group.endsWith(FLUX_SUFFIX));
}

export function argoInstalled(categories: Category[]): boolean {
  return descriptors(categories).some((descriptor) => {
    if (descriptor.group !== ARGO_GROUP) {
      return false;
    }
    return descriptor.kind === 'Application';
  });
}

export function gitopsAbsence(view: View, categories: Category[]): string | null {
  const engine = ENGINE_BY_VIEW[view];
  if (engine === undefined) {
    return null;
  }
  if (engine === 'flux' && fluxInstalled(categories)) {
    return null;
  }
  if (engine === 'argo' && argoInstalled(categories)) {
    return null;
  }
  return CLUSTER_ABSENCE[engine];
}

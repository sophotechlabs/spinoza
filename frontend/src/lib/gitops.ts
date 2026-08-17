import type { Category, ResourceDescriptor } from './types';

export const FLUX_SUFFIX = '.toolkit.fluxcd.io';

export const ARGO_GROUP = 'argoproj.io';

const ARGO_KINDS = ['Application', 'ApplicationSet', 'AppProject'];

function descriptors(categories: Category[]): ResourceDescriptor[] {
  return categories.flatMap((category) => category.resources);
}

export function fluxInstalled(categories: Category[]): boolean {
  return descriptors(categories).some((descriptor) => descriptor.group.endsWith(FLUX_SUFFIX));
}

export function argoInstalled(categories: Category[]): boolean {
  return argoTypes(categories).length > 0;
}

export function argoTypes(categories: Category[]): ResourceDescriptor[] {
  const found = descriptors(categories).filter((descriptor) => {
    if (descriptor.group !== ARGO_GROUP) {
      return false;
    }
    return ARGO_KINDS.includes(descriptor.kind);
  });
  return found.sort(
    (left, right) => ARGO_KINDS.indexOf(left.kind) - ARGO_KINDS.indexOf(right.kind),
  );
}

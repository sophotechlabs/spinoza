import type { Category, ResourceDescriptor } from './types';

const FLUX_SUFFIX = '.toolkit.fluxcd.io';

const ARGO_GROUP = 'argoproj.io';

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

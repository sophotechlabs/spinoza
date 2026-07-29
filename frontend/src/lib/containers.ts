import type { ContainerState } from './types';

export function isDebugContainer(container: ContainerState): boolean {
  return container.ephemeral === true;
}

export function containerNames(containers: ContainerState[]): string[] {
  const regular = containers.filter((container) => !container.init && !isDebugContainer(container));
  const ephemeral = containers.filter(isDebugContainer);
  const init = containers.filter((container) => container.init);
  return [...regular, ...ephemeral, ...init].map((container) => container.name);
}

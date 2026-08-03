import type { Row } from './types';
import { containerNames } from './containers';

export interface PodTarget {
  namespace: string;
  name: string;
  containers: string[];
}

export function podTarget(row: Row | null): PodTarget | null {
  if (row === null) {
    return null;
  }
  if (row.containers === undefined) {
    return null;
  }
  if (row.containers.length === 0) {
    return null;
  }
  return {
    namespace: row.namespace,
    name: row.name,
    containers: containerNames(row.containers),
  };
}

export function firstContainer(pod: PodTarget | null): string {
  if (pod === null) {
    return '';
  }
  if (pod.containers.length === 0) {
    return '';
  }
  return pod.containers[0];
}

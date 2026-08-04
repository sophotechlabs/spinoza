import type { ObjectDetail, Row } from './types';
import type { Selection } from './refs';
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

function podFromDetail(detail: ObjectDetail | null): PodTarget | null {
  if (detail === null) {
    return null;
  }
  if (detail.kind !== 'Pod') {
    return null;
  }
  const names = detail.pod?.containers ?? [];
  if (names.length === 0) {
    return null;
  }
  return { namespace: detail.namespace, name: detail.name, containers: names };
}

export function podFor(selection: Selection | null, detail: ObjectDetail | null): PodTarget | null {
  if (selection === null) {
    return null;
  }
  const live = podTarget(selection.row);
  if (live !== null) {
    return live;
  }
  return podFromDetail(detail);
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

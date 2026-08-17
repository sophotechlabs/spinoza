import type { FluxGroup, FluxResource } from './types';

export function reportingOf(resources: FluxResource[]): number {
  return resources.filter((resource) => resource.ready !== '').length;
}

export function readyOf(resources: FluxResource[]): number {
  return resources.filter((resource) => resource.ready === 'True').length;
}

export function readySummary(ready: number, reporting: number, total: number): string {
  if (total === 0) {
    return 'no resources';
  }
  if (reporting === total) {
    return `${ready}/${total} ready`;
  }
  return `${ready}/${reporting} ready, ${total - reporting} no status`;
}

export function groupSummary(group: FluxGroup): string {
  return readySummary(group.ready, group.reporting, group.total);
}

export function allReady(ready: number, reporting: number, total: number): boolean {
  if (total === 0) {
    return false;
  }
  if (reporting === 0) {
    return false;
  }
  return ready === reporting;
}

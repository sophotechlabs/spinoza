import type { FluxResource, ReadyState } from './types';

export function readyDot(ready: ReadyState): string {
  if (ready === 'True') {
    return 'bg-ok-solid';
  }
  if (ready === 'False') {
    return 'bg-error-solid';
  }
  return 'bg-idle-solid';
}

export function readyText(ready: ReadyState): string {
  if (ready === 'True') {
    return 'text-ok';
  }
  if (ready === 'False') {
    return 'text-error';
  }
  return 'text-fg-muted';
}

export function readyLabel(ready: ReadyState): string {
  if (ready === 'True') {
    return 'Ready';
  }
  if (ready === 'False') {
    return 'Not ready';
  }
  if (ready === '') {
    return 'No status';
  }
  return 'Unknown';
}

export function statusDot(resource: FluxResource): string {
  if (resource.suspended) {
    return 'bg-warn-solid';
  }
  return readyDot(resource.ready);
}

export function statusText(resource: FluxResource): string {
  if (resource.suspended) {
    return 'text-warn';
  }
  return readyText(resource.ready);
}

export function statusLabel(resource: FluxResource): string {
  if (resource.suspended) {
    return 'Suspended';
  }
  return readyLabel(resource.ready);
}

export function created(createdAt: string): string {
  if (createdAt === '') {
    return '';
  }
  return createdAt.slice(0, 10);
}

export function latestColor(resource: FluxResource): string {
  if (resource.outdated === true) {
    return 'text-warn';
  }
  return 'text-fg-muted';
}

export function latestTitle(resource: FluxResource): string {
  if (resource.latest === undefined || resource.latest === '') {
    return '';
  }
  if (resource.outdated === true) {
    return `${resource.revision} → ${resource.latest} available`;
  }
  return 'up to date';
}

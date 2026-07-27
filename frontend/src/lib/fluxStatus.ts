import type { FluxResource } from './types';

export function readyDot(ready: string): string {
  if (ready === 'True') {
    return 'bg-green-500';
  }
  if (ready === 'False') {
    return 'bg-red-500';
  }
  return 'bg-neutral-500';
}

export function readyText(ready: string): string {
  if (ready === 'True') {
    return 'text-green-400';
  }
  if (ready === 'False') {
    return 'text-red-400';
  }
  return 'text-neutral-500';
}

export function readyLabel(ready: string): string {
  if (ready === 'True') {
    return 'Ready';
  }
  if (ready === 'False') {
    return 'Not ready';
  }
  return 'Unknown';
}

export function statusDot(resource: FluxResource): string {
  if (resource.suspended) {
    return 'bg-amber-500';
  }
  return readyDot(resource.ready);
}

export function statusText(resource: FluxResource): string {
  if (resource.suspended) {
    return 'text-amber-400';
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
    return 'text-amber-400';
  }
  return 'text-neutral-600';
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

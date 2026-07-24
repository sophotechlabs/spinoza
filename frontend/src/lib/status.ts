import type { ContainerState } from './types';

const BAD_WAITING = ['CrashLoop', 'Err', 'BackOff', 'Invalid', 'Failed'];

function isBadWaiting(reason: string): boolean {
  for (const bad of BAD_WAITING) {
    if (reason.includes(bad)) {
      return true;
    }
  }
  return false;
}

export function containerColor(container: ContainerState): string {
  if (container.state === 'running' && container.ready) {
    return 'bg-green-500';
  }
  if (container.state === 'terminated' && container.reason === 'Completed') {
    return 'bg-neutral-500';
  }
  if (container.state === 'terminated') {
    return 'bg-red-500';
  }
  if (container.state === 'waiting' && isBadWaiting(container.reason ?? '')) {
    return 'bg-red-500';
  }
  return 'bg-yellow-500';
}

export function containerTitle(container: ContainerState): string {
  let title = `${container.name}: ${container.state}`;
  if (container.reason !== undefined && container.reason !== '') {
    title = `${title} (${container.reason})`;
  }
  if (container.restarts > 0) {
    title = `${title} · ${container.restarts} restarts`;
  }
  return title;
}

export function ratioColor(value: string): string {
  const parts = value.split('/');
  if (parts.length !== 2) {
    return 'text-neutral-200';
  }
  if (parts[0] === parts[1]) {
    return 'text-green-400';
  }
  if (parts[0] === '0') {
    return 'text-red-400';
  }
  return 'text-yellow-400';
}

export function restartColor(value: string): string {
  const count = Number(value);
  if (Number.isNaN(count) || count === 0) {
    return 'text-neutral-400';
  }
  if (count >= 5) {
    return 'text-red-400';
  }
  return 'text-yellow-400';
}

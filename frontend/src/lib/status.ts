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
    return 'bg-ok-solid';
  }
  if (container.state === 'terminated' && container.reason === 'Completed') {
    return 'bg-idle-solid';
  }
  if (container.state === 'terminated') {
    return 'bg-error-solid';
  }
  if (container.state === 'waiting' && isBadWaiting(container.reason ?? '')) {
    return 'bg-error-solid';
  }
  return 'bg-warn-solid';
}

export function containerTitle(container: ContainerState): string {
  let title = `${container.name}: ${container.state}`;
  if (container.reason !== undefined && container.reason !== '') {
    title = `${title} (${container.reason})`;
  }
  if (container.restarts > 0) {
    title = `${title}, ${container.restarts} restarts`;
  }
  return title;
}

const GOOD_STATUS = ['Running', 'Ready', 'Active', 'Bound', 'Available', 'Healthy'];
const SETTLED_STATUS = ['Succeeded', 'Completed'];
const BAD_STATUS = ['Failed', 'Error', 'Evicted', 'Lost', 'NotReady', 'Unhealthy', 'OOMKilled'];

export function statusColor(value: string): string {
  if (value === '') {
    return 'text-fg-faint';
  }
  if (GOOD_STATUS.includes(value)) {
    return 'text-ok';
  }
  if (SETTLED_STATUS.includes(value)) {
    return 'text-fg-muted';
  }
  if (BAD_STATUS.includes(value)) {
    return 'text-error';
  }
  if (isBadWaiting(value)) {
    return 'text-error';
  }
  return 'text-warn';
}

export function ratioColor(value: string): string {
  const parts = value.split('/');
  if (parts.length !== 2) {
    return 'text-fg';
  }
  if (parts[0] === parts[1]) {
    return 'text-ok';
  }
  if (parts[0] === '0') {
    return 'text-error';
  }
  return 'text-warn';
}

export function restartColor(value: string): string {
  const count = Number(value);
  if (Number.isNaN(count) || count === 0) {
    return 'text-fg-muted';
  }
  if (count >= 5) {
    return 'text-error';
  }
  return 'text-warn';
}

const ALARMING_WHEN_TRUE = [
  'Pressure',
  'Unavailable',
  'Degraded',
  'Failed',
  'Failure',
  'Error',
  'Stalled',
  'Disruption',
];

const NEITHER_WAY_IS_TROUBLE = ['Reconciling', 'Issuing'];

export function alarmingWhenTrue(type: string): boolean {
  return ALARMING_WHEN_TRUE.some((word) => type.includes(word));
}

export function conditionColor(type: string, status: string): string {
  if (NEITHER_WAY_IS_TROUBLE.includes(type)) {
    return 'text-fg-muted';
  }
  if (status !== 'True' && status !== 'False') {
    return 'text-fg-muted';
  }
  if (alarmingWhenTrue(type) === (status === 'True')) {
    return 'text-error';
  }
  return 'text-ok';
}

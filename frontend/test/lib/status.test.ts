import { describe, expect, it } from 'vitest';
import type { ContainerState } from '../../src/lib/types';
import {
  containerColor,
  containerTitle,
  ratioColor,
  restartColor,
  statusColor,
} from '../../src/lib/status';

function container(overrides: Partial<ContainerState>): ContainerState {
  const base: ContainerState = {
    name: 'c',
    state: 'running',
    ready: true,
    restarts: 0,
    init: false,
  };
  return { ...base, ...overrides };
}

describe('containerColor', () => {
  it('is green for a running, ready container', () => {
    expect(containerColor(container({ state: 'running', ready: true }))).toBe('bg-green-500');
  });

  it('is yellow for a running but not-ready container', () => {
    expect(containerColor(container({ state: 'running', ready: false }))).toBe('bg-yellow-500');
  });

  it('is neutral for a completed terminated container', () => {
    expect(containerColor(container({ state: 'terminated', reason: 'Completed' }))).toBe(
      'bg-neutral-500',
    );
  });

  it('is red for a terminated container that did not complete', () => {
    expect(containerColor(container({ state: 'terminated', reason: 'Error' }))).toBe('bg-red-500');
  });

  it('is red for a waiting container with a crash reason', () => {
    expect(containerColor(container({ state: 'waiting', reason: 'CrashLoopBackOff' }))).toBe(
      'bg-red-500',
    );
  });

  it('is yellow for a waiting container that is just starting', () => {
    expect(containerColor(container({ state: 'waiting', reason: 'ContainerCreating' }))).toBe(
      'bg-yellow-500',
    );
  });

  it('is yellow for a waiting container with no reason', () => {
    expect(containerColor(container({ state: 'waiting' }))).toBe('bg-yellow-500');
  });
});

describe('containerTitle', () => {
  it('includes only name and state for a healthy container', () => {
    expect(containerTitle(container({ name: 'app', state: 'running' }))).toBe('app: running');
  });

  it('adds the reason and restart count when present', () => {
    expect(
      containerTitle(
        container({ name: 'app', state: 'waiting', reason: 'CrashLoopBackOff', restarts: 4 }),
      ),
    ).toBe('app: waiting (CrashLoopBackOff) · 4 restarts');
  });
});

describe('ratioColor', () => {
  it('is green when the two parts match', () => {
    expect(ratioColor('3/3')).toBe('text-green-400');
  });

  it('is red when the ready part is zero', () => {
    expect(ratioColor('0/3')).toBe('text-red-400');
  });

  it('is yellow for a partial ratio', () => {
    expect(ratioColor('2/3')).toBe('text-yellow-400');
  });

  it('is neutral when the value is not a two-part ratio', () => {
    expect(ratioColor('5')).toBe('text-neutral-200');
    expect(ratioColor('1/2/3')).toBe('text-neutral-200');
  });
});

describe('restartColor', () => {
  it('is neutral for zero or unparseable counts', () => {
    expect(restartColor('0')).toBe('text-neutral-400');
    expect(restartColor('nope')).toBe('text-neutral-400');
  });

  it('is yellow for a low count', () => {
    expect(restartColor('3')).toBe('text-yellow-400');
  });

  it('is red for a high count', () => {
    expect(restartColor('9')).toBe('text-red-400');
  });
});

describe('statusColor', () => {
  it('greens the healthy states', () => {
    expect(statusColor('Running')).toBe('text-green-400');
    expect(statusColor('Ready')).toBe('text-green-400');
    expect(statusColor('Active')).toBe('text-green-400');
    expect(statusColor('Bound')).toBe('text-green-400');
  });

  it('mutes states that finished on purpose', () => {
    expect(statusColor('Succeeded')).toBe('text-neutral-400');
    expect(statusColor('Completed')).toBe('text-neutral-400');
  });

  it('reds the broken states', () => {
    expect(statusColor('Failed')).toBe('text-red-400');
    expect(statusColor('NotReady')).toBe('text-red-400');
    expect(statusColor('Evicted')).toBe('text-red-400');
  });

  it('reds the container failure reasons', () => {
    expect(statusColor('CrashLoopBackOff')).toBe('text-red-400');
    expect(statusColor('ImagePullBackOff')).toBe('text-red-400');
    expect(statusColor('ErrImagePull')).toBe('text-red-400');
    expect(statusColor('InvalidImageName')).toBe('text-red-400');
  });

  it('warns on anything still in flight', () => {
    expect(statusColor('Pending')).toBe('text-yellow-400');
    expect(statusColor('Terminating')).toBe('text-yellow-400');
    expect(statusColor('Unknown')).toBe('text-yellow-400');
    expect(statusColor('ArtifactFailed')).toBe('text-red-400');
  });

  it('leaves an empty status unstyled', () => {
    expect(statusColor('')).toBe('text-neutral-600');
  });
});

describe('a cordoned node', () => {
  it('reads as a warning rather than healthy', () => {
    expect(statusColor('Ready')).toBe('text-green-400');
    expect(statusColor('Ready,SchedulingDisabled')).toBe('text-yellow-400');
  });
});

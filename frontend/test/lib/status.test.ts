import { describe, expect, it } from 'vitest';
import type { ContainerState } from '../../src/lib/types';
import {
  alarmingWhenTrue,
  conditionColor,
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
    expect(containerColor(container({ state: 'running', ready: true }))).toBe('bg-ok-solid');
  });

  it('is yellow for a running but not-ready container', () => {
    expect(containerColor(container({ state: 'running', ready: false }))).toBe('bg-warn-solid');
  });

  it('is neutral for a completed terminated container', () => {
    expect(containerColor(container({ state: 'terminated', reason: 'Completed' }))).toBe(
      'bg-idle-solid',
    );
  });

  it('is red for a terminated container that did not complete', () => {
    expect(containerColor(container({ state: 'terminated', reason: 'Error' }))).toBe(
      'bg-error-solid',
    );
  });

  it('is red for a waiting container with a crash reason', () => {
    expect(containerColor(container({ state: 'waiting', reason: 'CrashLoopBackOff' }))).toBe(
      'bg-error-solid',
    );
  });

  it('is yellow for a waiting container that is just starting', () => {
    expect(containerColor(container({ state: 'waiting', reason: 'ContainerCreating' }))).toBe(
      'bg-warn-solid',
    );
  });

  it('is yellow for a waiting container with no reason', () => {
    expect(containerColor(container({ state: 'waiting' }))).toBe('bg-warn-solid');
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
    ).toBe('app: waiting (CrashLoopBackOff), 4 restarts');
  });
});

describe('ratioColor', () => {
  it('is green when the two parts match', () => {
    expect(ratioColor('3/3')).toBe('text-ok');
  });

  it('is red when the ready part is zero', () => {
    expect(ratioColor('0/3')).toBe('text-error');
  });

  it('is yellow for a partial ratio', () => {
    expect(ratioColor('2/3')).toBe('text-warn');
  });

  it('is neutral when the value is not a two-part ratio', () => {
    expect(ratioColor('5')).toBe('text-fg');
    expect(ratioColor('1/2/3')).toBe('text-fg');
  });
});

describe('restartColor', () => {
  it('is neutral for zero or unparseable counts', () => {
    expect(restartColor('0')).toBe('text-fg-muted');
    expect(restartColor('nope')).toBe('text-fg-muted');
  });

  it('is yellow for a low count', () => {
    expect(restartColor('3')).toBe('text-warn');
  });

  it('is red for a high count', () => {
    expect(restartColor('9')).toBe('text-error');
  });
});

describe('statusColor', () => {
  it('greens the healthy states', () => {
    expect(statusColor('Running')).toBe('text-ok');
    expect(statusColor('Ready')).toBe('text-ok');
    expect(statusColor('Active')).toBe('text-ok');
    expect(statusColor('Bound')).toBe('text-ok');
  });

  it('mutes states that finished on purpose', () => {
    expect(statusColor('Succeeded')).toBe('text-fg-muted');
    expect(statusColor('Completed')).toBe('text-fg-muted');
  });

  it('reds the broken states', () => {
    expect(statusColor('Failed')).toBe('text-error');
    expect(statusColor('NotReady')).toBe('text-error');
    expect(statusColor('Evicted')).toBe('text-error');
  });

  it('reds the container failure reasons', () => {
    expect(statusColor('CrashLoopBackOff')).toBe('text-error');
    expect(statusColor('ImagePullBackOff')).toBe('text-error');
    expect(statusColor('ErrImagePull')).toBe('text-error');
    expect(statusColor('InvalidImageName')).toBe('text-error');
  });

  it('warns on anything still in flight', () => {
    expect(statusColor('Pending')).toBe('text-warn');
    expect(statusColor('Terminating')).toBe('text-warn');
    expect(statusColor('Unknown')).toBe('text-warn');
    expect(statusColor('ArtifactFailed')).toBe('text-error');
  });

  it('leaves an empty status unstyled', () => {
    expect(statusColor('')).toBe('text-fg-faint');
  });
});

describe('a cordoned node', () => {
  it('reads as a warning rather than healthy', () => {
    expect(statusColor('Ready')).toBe('text-ok');
    expect(statusColor('Ready,SchedulingDisabled')).toBe('text-warn');
  });
});

describe('which way a condition reads', () => {
  it('treats the node pressures as trouble only when true', () => {
    for (const type of ['DiskPressure', 'MemoryPressure', 'PIDPressure']) {
      expect(conditionColor(type, 'False')).toBe('text-ok');
      expect(conditionColor(type, 'True')).toBe('text-error');
    }
  });

  it('reads the usual conditions the other way round', () => {
    for (const type of ['Ready', 'Available', 'ContainersReady', 'PodScheduled']) {
      expect(conditionColor(type, 'True')).toBe('text-ok');
      expect(conditionColor(type, 'False')).toBe('text-error');
    }
  });

  it('knows the negative words a CRD might use', () => {
    expect(alarmingWhenTrue('NetworkUnavailable')).toBe(true);
    expect(alarmingWhenTrue('Degraded')).toBe(true);
    expect(alarmingWhenTrue('Stalled')).toBe(true);
    expect(alarmingWhenTrue('ImagePullFailed')).toBe(true);
    expect(alarmingWhenTrue('DisruptionTarget')).toBe(true);
    expect(alarmingWhenTrue('Ready')).toBe(false);
    expect(alarmingWhenTrue('Healthy')).toBe(false);
  });

  it('leaves the ones that report work in progress alone', () => {
    for (const type of ['Reconciling', 'Progressing', 'Issuing', 'Initialized']) {
      expect(conditionColor(type, 'True')).toBe('text-fg-muted');
      expect(conditionColor(type, 'False')).toBe('text-fg-muted');
    }
  });

  it('says nothing about a status it cannot read', () => {
    expect(conditionColor('Ready', 'Unknown')).toBe('text-fg-muted');
    expect(conditionColor('DiskPressure', '')).toBe('text-fg-muted');
  });
});

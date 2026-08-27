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

  it('leaves the ones that only report work in flight alone', () => {
    for (const type of ['Reconciling', 'Issuing']) {
      expect(conditionColor(type, 'True')).toBe('text-fg-muted');
      expect(conditionColor(type, 'False')).toBe('text-fg-muted');
    }
  });

  it('still calls out a deployment that stopped progressing', () => {
    expect(conditionColor('Progressing', 'True')).toBe('text-ok');
    expect(conditionColor('Progressing', 'False')).toBe('text-error');
    expect(conditionColor('Initialized', 'True')).toBe('text-ok');
  });

  // A GKE node carries two dozen of these. They sit at False all day on a node
  // with nothing wrong with it, and reading them as readiness conditions is what
  // painted a healthy node red twenty-two times over.
  it('reads a node problem detector the right way round', () => {
    const detectors = [
      'KernelDeadlock',
      'CorruptDockerOverlay2',
      'FrequentKubeletRestart',
      'FrequentUnregisterNetDevice',
      'GcfsSnapshotterUnhealthy',
      'ReadOnlyRootFileSystem',
      'ResourceExhausted',
      'SecondaryBootDiskMissingLayer',
      'XfsShutdown',
      'DeprecatedUsingV1Alpha2Cri',
      'CperHardwareErrorFatal',
    ];
    for (const type of detectors) {
      expect(conditionColor(type, 'False')).toBe('text-ok');
      expect(conditionColor(type, 'True')).toBe('text-error');
    }
  });

  it('reads a negation as the trouble it is', () => {
    expect(conditionColor('NetworkUnavailable', 'False')).toBe('text-ok');
    expect(conditionColor('NetworkUnavailable', 'True')).toBe('text-error');
    expect(conditionColor('GcfsdUnhealthy', 'False')).toBe('text-ok');
    expect(conditionColor('GcfsdUnhealthy', 'True')).toBe('text-error');
  });

  // A type can name a good thing and a failure of it at once. The failure is
  // what it is reporting, so that is what decides.
  it('lets the trouble win when a type names both', () => {
    expect(conditionColor('ReadyReplicasMissing', 'True')).toBe('text-error');
    expect(conditionColor('ReadyReplicasMissing', 'False')).toBe('text-ok');
    expect(conditionColor('AvailableProbeFailed', 'True')).toBe('text-error');
  });

  // A notice is neither good news nor bad, and a type spinoza has never seen is
  // not worth a guess. Grey says so; green and red would both be claims.
  it('stays out of it for a condition it does not recognise', () => {
    for (const type of ['KubeletConfigChanged', 'SysctlChanged', 'Swap', 'SomeCustomThing']) {
      expect(conditionColor(type, 'True')).toBe('text-fg-muted');
      expect(conditionColor(type, 'False')).toBe('text-fg-muted');
    }
  });

  it('says nothing about a status it cannot read', () => {
    expect(conditionColor('Ready', 'Unknown')).toBe('text-fg-muted');
    expect(conditionColor('DiskPressure', '')).toBe('text-fg-muted');
  });
});

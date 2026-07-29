import { describe, expect, it } from 'vitest';
import { containerNames, isDebugContainer } from '../../src/lib/containers';
import type { ContainerState } from '../../src/lib/types';

function container(overrides: Partial<ContainerState>): ContainerState {
  return {
    name: 'app',
    state: 'running',
    ready: true,
    restarts: 0,
    init: false,
    ...overrides,
  };
}

describe('containerNames', () => {
  it('puts workload containers first', () => {
    const names = containerNames([
      container({ name: 'setup', init: true }),
      container({ name: 'app' }),
    ]);

    expect(names).toEqual(['app', 'setup']);
  });

  it('puts debug containers after the workload but before init containers', () => {
    const names = containerNames([
      container({ name: 'setup', init: true }),
      container({ name: 'spinoza-debug-1', ephemeral: true }),
      container({ name: 'app' }),
      container({ name: 'sidecar' }),
    ]);

    expect(names).toEqual(['app', 'sidecar', 'spinoza-debug-1', 'setup']);
  });

  it('never treats a debug container as a workload container', () => {
    const names = containerNames([container({ name: 'spinoza-debug-1', ephemeral: true })]);

    expect(names).toEqual(['spinoza-debug-1']);
  });

  it('recognises a debug container', () => {
    expect(isDebugContainer(container({ ephemeral: true }))).toBe(true);
    expect(isDebugContainer(container({}))).toBe(false);
    expect(isDebugContainer(container({ init: true }))).toBe(false);
  });
});

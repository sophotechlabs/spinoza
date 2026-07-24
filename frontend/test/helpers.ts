import type { PodRow } from '../src/lib/types';

export function makePod(overrides: Partial<PodRow>): PodRow {
  const base: PodRow = {
    uid: 'uid-0',
    name: 'pod-0',
    namespace: 'default',
    phase: 'Running',
    ready: '1/1',
    restarts: 0,
    node: 'node-a',
    createdAt: '2026-07-01T00:00:00Z',
  };
  return { ...base, ...overrides };
}

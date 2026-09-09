import { describe, expect, it } from 'vitest';
import {
  argoSummary,
  argoSummaryLabel,
  healthClass,
  orDash,
  syncClass,
} from '../../src/lib/argoStatus';

describe('syncClass', () => {
  it('is calm when the app is synced', () => {
    expect(syncClass('Synced')).toContain('text-ok');
  });

  it('warns on any other sync state', () => {
    expect(syncClass('OutOfSync')).toContain('text-warn');
  });

  it('stays quiet for a resource with no sync state', () => {
    expect(syncClass('')).toContain('text-fg-muted');
  });
});

describe('healthClass', () => {
  it('is calm when the app is healthy', () => {
    expect(healthClass('Healthy')).toContain('text-ok');
  });

  it('is loud when the app is degraded or missing', () => {
    expect(healthClass('Degraded')).toContain('text-error');
    expect(healthClass('Missing')).toContain('text-error');
  });

  it('warns on a state in between', () => {
    expect(healthClass('Progressing')).toContain('text-warn');
  });

  it('stays quiet for a resource with no health', () => {
    expect(healthClass('')).toContain('text-fg-muted');
  });
});

describe('orDash', () => {
  it('leaves a value alone', () => {
    expect(orDash('Synced')).toBe('Synced');
  });

  it('marks an empty field with a dash', () => {
    expect(orDash('')).toBe('-');
  });
});

describe('what the Argo overview says before the table', () => {
  it('counts the applications, the synced ones and the healthy ones', () => {
    const summary = argoSummary([
      { sync: 'Synced', health: 'Healthy' },
      { sync: 'OutOfSync', health: 'Healthy' },
      { sync: 'Synced', health: 'Degraded' },
    ]);

    expect(summary).toEqual({ total: 3, synced: 2, healthy: 2 });
    expect(argoSummaryLabel(summary)).toBe('3 applications, 2 synced, 2 healthy');
  });

  it('says so plainly when there is nothing to summarise', () => {
    expect(argoSummaryLabel(argoSummary([]))).toBe('No applications on this cluster');
  });
});

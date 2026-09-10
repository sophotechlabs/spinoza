import { describe, expect, it } from 'vitest';
import { SEVERITY_ORDER, severityClass, severityLabel, severityRank } from '../../src/lib/severity';

describe('one severity scale', () => {
  it('runs from high to low and nothing else', () => {
    expect(SEVERITY_ORDER).toEqual(['high', 'medium', 'low']);
  });

  it('gives each level one colour, wherever it is shown', () => {
    expect(severityClass('high')).toBe('text-error');
    expect(severityClass('medium')).toBe('text-warn');
    expect(severityClass('low')).toBe('text-fg-muted');
  });

  it('keeps a level nobody declared quiet rather than loud', () => {
    expect(severityClass('whatever')).toBe('text-fg-muted');
    expect(severityLabel('whatever')).toBe('whatever');
  });

  it('names each level the same word the api uses', () => {
    expect(severityLabel('high')).toBe('high');
    expect(severityLabel('medium')).toBe('medium');
    expect(severityLabel('low')).toBe('low');
  });

  it('ranks high above medium above low', () => {
    expect(severityRank('high')).toBeLessThan(severityRank('medium'));
    expect(severityRank('medium')).toBeLessThan(severityRank('low'));
    expect(severityRank('whatever')).toBeGreaterThan(severityRank('low'));
  });
});

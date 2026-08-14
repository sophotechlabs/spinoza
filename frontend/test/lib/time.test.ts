import { describe, expect, it } from 'vitest';
import { ago, clock, since } from '../../src/lib/time';

const NOW = Date.parse('2026-08-11T12:00:00Z');

describe('how long ago something happened', () => {
  it('counts seconds under a minute', () => {
    expect(ago('2026-08-11T11:59:31Z', NOW)).toBe('29s');
  });

  it('counts minutes under an hour', () => {
    expect(ago('2026-08-11T11:05:00Z', NOW)).toBe('55m');
  });

  it('counts hours under a day', () => {
    expect(ago('2026-08-11T02:00:00Z', NOW)).toBe('10h');
  });

  it('counts days beyond that', () => {
    expect(ago('2026-08-01T12:00:00Z', NOW)).toBe('10d');
  });

  it('reads a clock ahead of us as just now', () => {
    expect(ago('2026-08-11T12:00:30Z', NOW)).toBe('0s');
  });

  it('says nothing for a missing stamp', () => {
    expect(ago('', NOW)).toBe('');
  });

  it('says nothing for a stamp it cannot read', () => {
    expect(ago('yesterday', NOW)).toBe('');
  });
});

describe('the units the elapsed time rolls into', () => {
  it('turns exactly a minute into minutes', () => {
    expect(since(60)).toBe('1m');
  });

  it('turns exactly an hour into hours', () => {
    expect(since(3600)).toBe('1h');
  });

  it('turns exactly a day into days', () => {
    expect(since(86400)).toBe('1d');
  });
});

describe('clock', () => {
  it('reads a stamp as the local wall time', () => {
    const at = new Date(2026, 7, 14, 9, 4, 7);

    expect(clock(at.toISOString())).toBe('09:04:07');
  });

  it('says nothing for a stamp it cannot read', () => {
    expect(clock('not a time')).toBe('');
  });
});

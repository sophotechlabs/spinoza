import { describe, expect, it } from 'vitest';
import {
  created,
  latestColor,
  latestTitle,
  readyDot,
  readyLabel,
  readyText,
  statusDot,
  statusLabel,
  statusText,
} from '../../src/lib/fluxStatus';
import { makeFluxResource } from '../helpers';

describe('ready helpers', () => {
  it('maps ready to dot colors', () => {
    expect(readyDot('True')).toBe('bg-green-500');
    expect(readyDot('False')).toBe('bg-red-500');
    expect(readyDot('')).toBe('bg-neutral-500');
  });

  it('maps ready to text colors', () => {
    expect(readyText('True')).toBe('text-green-400');
    expect(readyText('False')).toBe('text-red-400');
    expect(readyText('')).toBe('text-neutral-500');
  });

  it('maps ready to labels', () => {
    expect(readyLabel('True')).toBe('Ready');
    expect(readyLabel('False')).toBe('Not ready');
    expect(readyLabel('')).toBe('No status');
    expect(readyLabel('Unknown')).toBe('Unknown');
  });
});

describe('status helpers', () => {
  it('reports suspended regardless of ready', () => {
    const resource = makeFluxResource({ suspended: true, ready: 'True' });
    expect(statusDot(resource)).toBe('bg-amber-500');
    expect(statusText(resource)).toBe('text-amber-400');
    expect(statusLabel(resource)).toBe('Suspended');
  });

  it('falls through to the ready state when not suspended', () => {
    const resource = makeFluxResource({ suspended: false, ready: 'False' });
    expect(statusDot(resource)).toBe('bg-red-500');
    expect(statusText(resource)).toBe('text-red-400');
    expect(statusLabel(resource)).toBe('Not ready');
  });
});

describe('created', () => {
  it('slices the timestamp to a date and blanks the empty value', () => {
    expect(created('2026-07-24T09:00:00Z')).toBe('2026-07-24');
    expect(created('')).toBe('');
  });
});

describe('latest version presentation', () => {
  it('highlights an outdated release', () => {
    const resource = makeFluxResource({ revision: '6.14.0', latest: '6.15.1', outdated: true });
    expect(latestColor(resource)).toBe('text-amber-400');
    expect(latestTitle(resource)).toBe('6.14.0 → 6.15.1 available');
  });

  it('dims a current release', () => {
    const resource = makeFluxResource({ revision: '6.15.1', latest: '6.15.1' });
    expect(latestColor(resource)).toBe('text-neutral-600');
    expect(latestTitle(resource)).toBe('up to date');
  });

  it('has no title when the latest version is unknown', () => {
    expect(latestTitle(makeFluxResource({ latest: undefined }))).toBe('');
    expect(latestTitle(makeFluxResource({ latest: '' }))).toBe('');
  });
});

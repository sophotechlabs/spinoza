import { describe, expect, it } from 'vitest';
import {
  allReady,
  groupSummary,
  readyOf,
  readySummary,
  reportingOf,
} from '../../src/lib/readiness';
import { makeFluxResource } from '../helpers';

const resources = [
  makeFluxResource({ name: 'a', ready: 'True' }),
  makeFluxResource({ name: 'b', ready: 'False' }),
  makeFluxResource({ name: 'c', ready: '' }),
  makeFluxResource({ name: 'd', ready: 'Unknown' }),
];

describe('readiness counts', () => {
  it('counts only resources reporting True as ready', () => {
    expect(readyOf(resources)).toBe(1);
  });

  it('excludes resources with no Ready condition from the denominator', () => {
    expect(reportingOf(resources)).toBe(3);
  });

  it('keeps an explicit Unknown in the denominator', () => {
    expect(reportingOf([makeFluxResource({ ready: 'Unknown' })])).toBe(1);
  });
});

describe('readySummary', () => {
  it('reads plainly when everything reports', () => {
    expect(readySummary(20, 20, 20)).toBe('20/20 ready');
    expect(readySummary(11, 13, 13)).toBe('11/13 ready');
  });

  it('separates out resources that carry no status', () => {
    expect(readySummary(11, 11, 13)).toBe('11/11 ready · 2 no status');
  });

  it('says so when there is nothing to report on', () => {
    expect(readySummary(0, 0, 0)).toBe('no resources');
  });

  it('summarises a group', () => {
    expect(
      groupSummary({ name: 'Sources', ready: 11, reporting: 11, total: 13, resources: [] }),
    ).toBe('11/11 ready · 2 no status');
  });
});

describe('allReady', () => {
  it('is true when every reporting resource is ready', () => {
    expect(allReady(11, 11, 13)).toBe(true);
    expect(allReady(20, 20, 20)).toBe(true);
  });

  it('is false when one reporting resource is not ready', () => {
    expect(allReady(10, 11, 13)).toBe(false);
  });

  it('is false when nothing reports at all', () => {
    expect(allReady(0, 0, 2)).toBe(false);
    expect(allReady(0, 0, 0)).toBe(false);
  });
});

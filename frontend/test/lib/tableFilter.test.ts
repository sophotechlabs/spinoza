import { describe, expect, it } from 'vitest';
import { NO_FILTER, filterRows, imposeFilter } from '../../src/lib/tableFilter';
import { makeRow } from '../helpers';

const rows = [
  makeRow({ uid: 'a', name: 'web-1', namespace: 'prod' }),
  makeRow({ uid: 'b', name: 'web-2', namespace: 'staging' }),
  makeRow({ uid: 'c', name: 'api-1', namespace: 'prod' }),
  makeRow({ uid: 'd', name: 'node-1', namespace: '' }),
];

describe('tableFilter', () => {
  it('counts every imposed filter, so the same text lands twice', () => {
    const once = imposeFilter(NO_FILTER, 'coredns');
    const twice = imposeFilter(once, 'coredns');

    expect(once).toEqual({ text: 'coredns', at: 1 });
    expect(twice).toEqual({ text: 'coredns', at: 2 });
  });

  it('returns everything with no query', () => {
    expect(filterRows(rows, '')).toHaveLength(4);
  });

  it('filters by name substring, case-insensitively', () => {
    expect(filterRows(rows, 'WEB').map((row) => row.name)).toEqual(['web-1', 'web-2']);
  });

  it('ignores surrounding whitespace in the query', () => {
    expect(filterRows(rows, '  api  ').map((row) => row.name)).toEqual(['api-1']);
  });

  it('returns nothing when nothing matches', () => {
    expect(filterRows(rows, 'nope')).toEqual([]);
  });
});

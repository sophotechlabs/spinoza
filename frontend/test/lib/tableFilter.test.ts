import { describe, expect, it } from 'vitest';
import { ALL_NAMESPACES, filterRows, namespacesOf } from '../../src/lib/tableFilter';
import { makeRow } from '../helpers';

const rows = [
  makeRow({ uid: 'a', name: 'web-1', namespace: 'prod' }),
  makeRow({ uid: 'b', name: 'web-2', namespace: 'staging' }),
  makeRow({ uid: 'c', name: 'api-1', namespace: 'prod' }),
  makeRow({ uid: 'd', name: 'node-1', namespace: '' }),
];

describe('tableFilter', () => {
  it('lists sorted, unique, non-empty namespaces', () => {
    expect(namespacesOf(rows)).toEqual(['prod', 'staging']);
  });

  it('returns everything with no filters', () => {
    expect(filterRows(rows, '', ALL_NAMESPACES)).toHaveLength(4);
  });

  it('filters by name substring, case-insensitively', () => {
    const got = filterRows(rows, 'WEB', ALL_NAMESPACES).map((row) => row.name);
    expect(got).toEqual(['web-1', 'web-2']);
  });

  it('ignores surrounding whitespace in the query', () => {
    expect(filterRows(rows, '  api  ', ALL_NAMESPACES).map((row) => row.name)).toEqual(['api-1']);
  });

  it('filters by namespace', () => {
    const got = filterRows(rows, '', 'prod').map((row) => row.name);
    expect(got).toEqual(['web-1', 'api-1']);
  });

  it('combines both filters', () => {
    expect(filterRows(rows, 'web', 'prod').map((row) => row.name)).toEqual(['web-1']);
  });

  it('returns nothing when nothing matches', () => {
    expect(filterRows(rows, 'nope', ALL_NAMESPACES)).toEqual([]);
  });
});

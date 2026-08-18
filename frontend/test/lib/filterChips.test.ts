import { describe, expect, it } from 'vitest';
import {
  chipKey,
  fieldFor,
  fieldKey,
  fieldsOf,
  filterRows,
  labelOf,
  matches,
  nameChips,
  parseChip,
} from '../../src/lib/filterChips';
import { makeColumns, makeRow } from '../helpers';

const columns = [{ name: 'Status' }, { name: 'Last seen', render: 'age' }];

const fields = fieldsOf(columns, true);

const rows = [
  makeRow({ uid: 'a', name: 'web-1', namespace: 'prod', cells: ['Running', '1h'] }),
  makeRow({ uid: 'b', name: 'web-2', namespace: 'staging', cells: ['Pending', '2m'] }),
  makeRow({ uid: 'c', name: 'api-1', namespace: 'prod', cells: ['Running', '3d'] }),
];

describe('the fields a kind can be filtered by', () => {
  it('starts with name and namespace, then the kind columns', () => {
    expect(fields.map((one) => one.key)).toEqual(['name', 'namespace', 'status', 'lastseen']);
  });

  it('leaves namespace out of a cluster-scoped kind', () => {
    expect(fieldsOf(columns, false).map((one) => one.key)).toEqual(['name', 'status', 'lastseen']);
  });

  it('strips spaces and case out of a column name', () => {
    expect(fieldKey('Last seen')).toBe('lastseen');
    expect(fieldKey('Up-to-date')).toBe('uptodate');
  });

  it('drops a column whose name has nothing to key on', () => {
    expect(fieldsOf(makeColumns(['·']), false).map((one) => one.key)).toEqual(['name']);
  });

  it('keeps the first of two columns that key the same', () => {
    const twice = fieldsOf(makeColumns(['Status', 'STATUS']), false);

    expect(twice.filter((one) => one.key === 'status')).toHaveLength(1);
  });

  it('does not let a column shadow the name field', () => {
    const shadowed = fieldsOf(makeColumns(['Name']), false);

    expect(shadowed).toHaveLength(1);
    expect(shadowed[0].cell).toBe(-1);
  });

  it('answers to ns as well as namespace', () => {
    expect(fieldFor(fields, 'ns')?.key).toBe('namespace');
  });

  it('does not know a field the kind has no column for', () => {
    expect(fieldFor(fields, 'node')).toBeNull();
  });
});

describe('reading a chip out of typed text', () => {
  it('takes bare text as a name', () => {
    expect(parseChip('web', fields)).toEqual({ field: 'name', value: 'web' });
  });

  it('ignores surrounding whitespace', () => {
    expect(parseChip('  web  ', fields)).toEqual({ field: 'name', value: 'web' });
  });

  it('has nothing to commit for empty text', () => {
    expect(parseChip('   ', fields)).toBeNull();
  });

  it('reads field:value for a known field', () => {
    expect(parseChip('status:Running', fields)).toEqual({ field: 'status', value: 'Running' });
  });

  it('reads the alias and the column label the same way', () => {
    expect(parseChip('ns: prod', fields)).toEqual({ field: 'namespace', value: 'prod' });
    expect(parseChip('Last seen:1h', fields)).toEqual({ field: 'lastseen', value: '1h' });
  });

  it('waits for a value before committing a field', () => {
    expect(parseChip('status:', fields)).toBeNull();
  });

  it('has nothing to narrow when a kind has no namespace to scope to', () => {
    const clusterScoped = fieldsOf(columns, false);

    expect(parseChip('ns:kube-system', clusterScoped)).toBeNull();
    expect(parseChip('namespace:kube-system', clusterScoped)).toBeNull();
  });

  it('keeps an unknown prefix as part of the name', () => {
    expect(parseChip('image:nginx', fields)).toEqual({ field: 'name', value: 'image:nginx' });
  });

  it('makes a name chip out of a search query, and nothing out of an empty one', () => {
    expect(nameChips(' coredns ')).toEqual([{ field: 'name', value: 'coredns' }]);
    expect(nameChips('  ')).toEqual([]);
  });

  it('keys a chip case-insensitively', () => {
    expect(chipKey({ field: 'name', value: 'WEB' })).toBe(chipKey({ field: 'name', value: 'web' }));
  });

  it('labels a chip with the column it came from', () => {
    expect(labelOf({ field: 'lastseen', value: '1h' }, fields)).toBe('Last seen');
  });

  it('falls back to the raw field when the kind has no such column', () => {
    expect(labelOf({ field: 'node', value: 'n1' }, fields)).toBe('node');
  });
});

describe('filtering rows by chips', () => {
  it('returns everything when there is nothing to match', () => {
    expect(filterRows(rows, [], fields)).toHaveLength(3);
  });

  it('matches a name substring, case-insensitively', () => {
    const found = filterRows(rows, [{ field: 'name', value: 'WEB' }], fields);

    expect(found.map((row) => row.name)).toEqual(['web-1', 'web-2']);
  });

  it('matches a namespace', () => {
    const found = filterRows(rows, [{ field: 'namespace', value: 'prod' }], fields);

    expect(found.map((row) => row.name)).toEqual(['web-1', 'api-1']);
  });

  it('matches a cell of the kind column', () => {
    const found = filterRows(rows, [{ field: 'status', value: 'run' }], fields);

    expect(found.map((row) => row.name)).toEqual(['web-1', 'api-1']);
  });

  it('narrows by every chip at once', () => {
    const found = filterRows(
      rows,
      [
        { field: 'name', value: 'web' },
        { field: 'status', value: 'Running' },
      ],
      fields,
    );

    expect(found.map((row) => row.name)).toEqual(['web-1']);
  });

  it('reads a missing cell as empty rather than crashing', () => {
    const short = [makeRow({ uid: 'd', name: 'short', namespace: 'prod', cells: [] })];

    expect(filterRows(short, [{ field: 'status', value: 'x' }], fields)).toEqual([]);
  });

  it('hides nothing for a chip the kind knows no field for', () => {
    expect(matches(rows[0], { field: 'node', value: 'anything' }, fields)).toBe(true);
  });
});

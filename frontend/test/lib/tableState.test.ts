import { beforeEach, describe, expect, it } from 'vitest';
import { readStored } from '../../src/lib/persist';
import {
  TABLE_STATE_KEY,
  columnLabel,
  emptyTableState,
  metricHeader,
  nextMetricSort,
  parseTables,
  readTableState,
  tableKey,
  writeTableState,
} from '../../src/lib/tableState';
import { makeDescriptor } from '../helpers';

describe('columnLabel', () => {
  it('uses the header when it is plain text', () => {
    expect(columnLabel('Ready', 'cell-0')).toBe('Ready');
  });

  it('falls back to the column id when the header is rendered', () => {
    expect(columnLabel(() => null, 'select')).toBe('select');
    expect(columnLabel(undefined, 'age')).toBe('age');
  });
});

describe('tableKey', () => {
  it('names a resource by its group, version and plural', () => {
    expect(tableKey(makeDescriptor({ group: 'apps', resource: 'deployments' }))).toBe(
      'apps/v1/deployments',
    );
  });

  it('is empty without a resource', () => {
    expect(tableKey(null)).toBe('');
  });
});

describe('parseTables', () => {
  it('is empty for nothing stored', () => {
    expect(parseTables(null)).toEqual({});
  });

  it('is empty for junk', () => {
    expect(parseTables('{not json')).toEqual({});
    expect(parseTables('"a string"')).toEqual({});
  });

  it('reads sorting, visibility and widths back', () => {
    const raw = JSON.stringify({
      'v1/pods': {
        sorting: [{ id: 'name', desc: true }],
        visibility: { age: false },
        sizing: { name: 300 },
      },
    });

    expect(parseTables(raw)['v1/pods']).toEqual({
      sorting: [{ id: 'name', desc: true }],
      visibility: { age: false },
      sizing: { name: 300 },
      bases: {},
    });
  });

  it('drops entries whose shape is wrong', () => {
    const raw = JSON.stringify({
      'v1/pods': {
        sorting: [{ id: '' }, { desc: true }, 'junk', { id: 'age' }],
        visibility: { age: 'nope', name: true },
        sizing: { name: 'wide', age: 40 },
      },
    });

    expect(parseTables(raw)['v1/pods']).toEqual({
      sorting: [{ id: 'age', desc: false }],
      visibility: { name: true },
      sizing: { age: 40 },
      bases: {},
    });
  });
});

describe('reading and writing a table', () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it('starts empty', () => {
    expect(readTableState('v1/pods')).toEqual(emptyTableState());
  });

  it('remembers what it wrote', () => {
    writeTableState('v1/pods', {
      sorting: [{ id: 'name', desc: false }],
      visibility: { age: false },
      sizing: { name: 200 },
      bases: {},
    });

    expect(readTableState('v1/pods').sorting).toEqual([{ id: 'name', desc: false }]);
    expect(readTableState('v1/pods').visibility).toEqual({ age: false });
  });

  it('keeps one resource kind apart from another', () => {
    writeTableState('v1/pods', {
      sorting: [{ id: 'name', desc: true }],
      visibility: {},
      sizing: {},
      bases: {},
    });
    writeTableState('v1/nodes', {
      sorting: [],
      visibility: { age: false },
      sizing: {},
      bases: {},
    });

    expect(readTableState('v1/pods').sorting).toHaveLength(1);
    expect(readTableState('v1/nodes').sorting).toHaveLength(0);
    expect(readTableState('v1/nodes').visibility).toEqual({ age: false });
  });

  it('shrugs off storage that refuses to read', () => {
    const broken = {
      getItem: () => {
        throw new Error('denied');
      },
      setItem: () => undefined,
    };
    const original = window.localStorage;
    Object.defineProperty(window, 'localStorage', { configurable: true, value: broken });

    expect(readTableState('v1/pods')).toEqual(emptyTableState());

    Object.defineProperty(window, 'localStorage', { configurable: true, value: original });
  });

  it('shrugs off storage that refuses to write', () => {
    const broken = {
      getItem: () => null,
      setItem: () => {
        throw new Error('quota');
      },
    };
    const original = window.localStorage;
    Object.defineProperty(window, 'localStorage', { configurable: true, value: broken });

    expect(() => {
      writeTableState('v1/pods', emptyTableState());
    }).not.toThrow();

    Object.defineProperty(window, 'localStorage', { configurable: true, value: original });
  });

  it('stores nothing without a resource', () => {
    writeTableState('', {
      sorting: [{ id: 'name', desc: true }],
      visibility: {},
      sizing: {},
      bases: {},
    });

    expect(readStored(TABLE_STATE_KEY)).toBeNull();
    expect(readTableState('')).toEqual(emptyTableState());
  });
});

describe('nextMetricSort', () => {
  it('starts at most consumed', () => {
    expect(nextMetricSort('memory', [], 'used')).toEqual({
      sorting: [{ id: 'memory', desc: true }],
      basis: 'used',
    });
  });

  it('turns most consumed into least consumed', () => {
    expect(nextMetricSort('memory', [{ id: 'memory', desc: true }], 'used')).toEqual({
      sorting: [{ id: 'memory', desc: false }],
      basis: 'used',
    });
  });

  it('moves to the largest once both directions of used are spent', () => {
    expect(nextMetricSort('memory', [{ id: 'memory', desc: false }], 'used')).toEqual({
      sorting: [{ id: 'memory', desc: true }],
      basis: 'total',
    });
  });

  it('turns the largest into the smallest', () => {
    expect(nextMetricSort('memory', [{ id: 'memory', desc: true }], 'total')).toEqual({
      sorting: [{ id: 'memory', desc: false }],
      basis: 'total',
    });
  });

  it('comes back round to unsorted', () => {
    expect(nextMetricSort('memory', [{ id: 'memory', desc: false }], 'total')).toEqual({
      sorting: [],
      basis: 'used',
    });
  });

  it('starts over when another column holds the sort', () => {
    expect(nextMetricSort('memory', [{ id: 'name', desc: false }], 'total')).toEqual({
      sorting: [{ id: 'memory', desc: true }],
      basis: 'used',
    });
  });
});

describe('metricHeader', () => {
  it('names the column while it sorts on what is used', () => {
    expect(metricHeader('Memory', true, 'used')).toBe('Memory');
  });

  it('says so while it sorts on the whole node', () => {
    expect(metricHeader('Memory', true, 'total')).toBe('Memory total');
  });

  it('says nothing extra while the column is not sorted', () => {
    expect(metricHeader('Memory', false, 'total')).toBe('Memory');
  });
});

describe('the remembered basis', () => {
  it('survives a reload', () => {
    writeTableState('v1/nodes', {
      sorting: [{ id: 'memory', desc: true }],
      visibility: {},
      sizing: {},
      bases: { memory: 'total' },
    });

    expect(readTableState('v1/nodes').bases).toEqual({ memory: 'total' });
  });

  it('takes nothing it does not recognise', () => {
    const parsed = parseTables(
      JSON.stringify({ 'v1/nodes': { bases: { memory: 'sideways', cpu: 'total' } } }),
    );

    expect(parsed['v1/nodes'].bases).toEqual({ cpu: 'total' });
  });
});

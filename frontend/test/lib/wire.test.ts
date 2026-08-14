import { describe, expect, it } from 'vitest';
import {
  asBoolean,
  asList,
  asNumber,
  asRecord,
  asString,
  listOf,
  numberMap,
  optionalNumberMap,
  oneOf,
  optionalBoolean,
  optionalListOf,
  optionalNumber,
  optionalString,
  recordMap,
  stringList,
  stringMap,
} from '../../src/lib/wire';

describe('asRecord', () => {
  it('keeps an object and rejects everything else', () => {
    expect(asRecord({ a: 1 })).toEqual({ a: 1 });
    expect(asRecord(null)).toEqual({});
    expect(asRecord([1, 2])).toEqual({});
    expect(asRecord('x')).toEqual({});
  });
});

describe('asList', () => {
  it('keeps an array and rejects everything else', () => {
    expect(asList([1, 2])).toEqual([1, 2]);
    expect(asList({ a: 1 })).toEqual([]);
    expect(asList(null)).toEqual([]);
  });
});

describe('asString', () => {
  it('keeps a string and falls back to empty', () => {
    expect(asString('a')).toBe('a');
    expect(asString(7)).toBe('');
    expect(asString(undefined)).toBe('');
  });
});

describe('optionalString', () => {
  it('keeps a string and drops anything else', () => {
    expect(optionalString('a')).toBe('a');
    expect(optionalString(7)).toBeUndefined();
  });
});

describe('asNumber', () => {
  it('keeps a finite number and falls back to zero', () => {
    expect(asNumber(7)).toBe(7);
    expect(asNumber(Number.NaN)).toBe(0);
    expect(asNumber(Number.POSITIVE_INFINITY)).toBe(0);
    expect(asNumber('7')).toBe(0);
  });
});

describe('optionalNumber', () => {
  it('keeps a finite number and drops anything else', () => {
    expect(optionalNumber(7)).toBe(7);
    expect(optionalNumber(Number.NaN)).toBeUndefined();
    expect(optionalNumber('7')).toBeUndefined();
  });
});

describe('asBoolean', () => {
  it('is true only for a literal true', () => {
    expect(asBoolean(true)).toBe(true);
    expect(asBoolean('true')).toBe(false);
    expect(asBoolean(1)).toBe(false);
  });
});

describe('optionalBoolean', () => {
  it('keeps both booleans and drops anything else', () => {
    expect(optionalBoolean(false)).toBe(false);
    expect(optionalBoolean(true)).toBe(true);
    expect(optionalBoolean('yes')).toBeUndefined();
  });
});

describe('oneOf', () => {
  it('accepts a known value and falls back on an unknown one', () => {
    const allowed = ['running', 'failed'] as const;
    expect(oneOf('running', allowed, 'failed')).toBe('running');
    expect(oneOf('starting', allowed, 'failed')).toBe('failed');
    expect(oneOf(3, allowed, 'failed')).toBe('failed');
  });
});

describe('stringMap', () => {
  it('keeps string entries and drops the rest', () => {
    expect(stringMap({ a: 'x', b: 3 })).toEqual({ a: 'x' });
    expect(stringMap(null)).toBeUndefined();
    expect(stringMap([1])).toBeUndefined();
    expect(stringMap('x')).toBeUndefined();
  });
});

describe('numberMap', () => {
  it('keeps finite number entries and drops the rest', () => {
    expect(numberMap({ a: 3, b: 'x', c: Number.NaN })).toEqual({ a: 3 });
    expect(numberMap(null)).toEqual({});
  });
});

describe('listOf', () => {
  it('parses each item through the given shape', () => {
    expect(listOf([{ n: 1 }, 'junk'], (item) => asNumber(item.n))).toEqual([1, 0]);
  });
});

describe('optionalListOf', () => {
  it('stays undefined when the field is absent', () => {
    expect(optionalListOf(undefined, (item) => asNumber(item.n))).toBeUndefined();
    expect(optionalListOf([{ n: 2 }], (item) => asNumber(item.n))).toEqual([2]);
  });
});

describe('recordMap', () => {
  it('parses every value of a keyed object', () => {
    expect(recordMap({ a: { n: 1 }, b: 'junk' }, (item) => asNumber(item.n))).toEqual({
      a: 1,
      b: 0,
    });
  });
});

describe('optionalNumberMap', () => {
  it('reads a map of numbers', () => {
    expect(optionalNumberMap({ '/v1/pods': 3 })).toEqual({ '/v1/pods': 3 });
  });

  it('is absent rather than empty when the field is missing', () => {
    expect(optionalNumberMap(undefined)).toBeUndefined();
  });

  it('refuses null and arrays', () => {
    expect(optionalNumberMap(null)).toBeUndefined();
    expect(optionalNumberMap([1, 2])).toBeUndefined();
  });

  it('drops entries that are not finite numbers', () => {
    expect(optionalNumberMap({ a: 1, b: 'two', c: Number.NaN })).toEqual({ a: 1 });
  });
});

describe('stringList', () => {
  it('keeps the strings in order', () => {
    expect(stringList(['6.15.1', '6.14.0'])).toEqual(['6.15.1', '6.14.0']);
  });

  it('drops entries that are not strings', () => {
    expect(stringList(['a', 1, null, 'b'])).toEqual(['a', 'b']);
  });

  it('reads anything that is not a list as empty', () => {
    expect(stringList(undefined)).toEqual([]);
    expect(stringList('6.15.1')).toEqual([]);
  });
});

import { describe, expect, it } from 'vitest';
import {
  asBoolean,
  asList,
  asNumber,
  asRecord,
  asString,
  listOf,
  numberMap,
  oneOf,
  optionalBoolean,
  optionalListOf,
  optionalNumber,
  optionalString,
  recordMap,
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

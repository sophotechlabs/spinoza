import { beforeEach, describe, expect, it } from 'vitest';
import { readStored } from '../../src/lib/persist';
import {
  GITOPS_SECTION,
  SIDEBAR_STATE_KEY,
  parseSections,
  readSections,
  sectionOpen,
  writeSections,
} from '../../src/lib/sidebarState';

describe('sectionOpen', () => {
  it('keeps resource categories shut until the user opens one', () => {
    expect(sectionOpen({}, 'Workloads')).toBe(false);
  });

  it('keeps the GitOps section open until the user shuts it', () => {
    expect(sectionOpen({}, GITOPS_SECTION)).toBe(true);
  });

  it('follows what was stored, whichever way it goes', () => {
    expect(sectionOpen({ Workloads: true }, 'Workloads')).toBe(true);
    expect(sectionOpen({ [GITOPS_SECTION]: false }, GITOPS_SECTION)).toBe(false);
  });
});

describe('parseSections', () => {
  it('is empty for nothing stored', () => {
    expect(parseSections(null)).toEqual({});
  });

  it('is empty for junk', () => {
    expect(parseSections('{not json')).toEqual({});
    expect(parseSections('"a string"')).toEqual({});
    expect(parseSections('null')).toEqual({});
    expect(parseSections('[1,2]')).toEqual({});
  });

  it('keeps boolean entries and drops the rest', () => {
    expect(parseSections('{"Workloads":true,"Config":"yes","GitOps":false}')).toEqual({
      Workloads: true,
      GitOps: false,
    });
  });
});

describe('reading and writing the sidebar', () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it('starts with nothing remembered', () => {
    expect(readSections()).toEqual({});
  });

  it('remembers what it wrote', () => {
    writeSections({ Workloads: true });

    expect(readSections()).toEqual({ Workloads: true });
    expect(readStored(SIDEBAR_STATE_KEY)).toBe('{"Workloads":true}');
  });

  it('shrugs off storage that refuses to read', () => {
    const original = window.localStorage;
    Object.defineProperty(window, 'localStorage', {
      configurable: true,
      value: {
        getItem: () => {
          throw new Error('denied');
        },
      },
    });

    expect(readSections()).toEqual({});

    Object.defineProperty(window, 'localStorage', { configurable: true, value: original });
  });

  it('shrugs off storage that refuses to write', () => {
    const original = window.localStorage;
    Object.defineProperty(window, 'localStorage', {
      configurable: true,
      value: {
        setItem: () => {
          throw new Error('quota');
        },
      },
    });

    expect(() => {
      writeSections({ Workloads: true });
    }).not.toThrow();

    Object.defineProperty(window, 'localStorage', { configurable: true, value: original });
  });
});

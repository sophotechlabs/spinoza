import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  applyTheme,
  parseTheme,
  readTheme,
  resolveTheme,
  systemTheme,
  watchSystemTheme,
  writeTheme,
  THEME_KEY,
} from '../../src/lib/theme';
import { emitSystemDark, setSystemDark } from '../helpers';

afterEach(() => {
  vi.restoreAllMocks();
  window.localStorage.clear();
  setSystemDark(false);
  delete document.documentElement.dataset.theme;
});

describe('parseTheme', () => {
  it('accepts every known preference', () => {
    expect(parseTheme('dark')).toBe('dark');
    expect(parseTheme('light')).toBe('light');
    expect(parseTheme('system')).toBe('system');
  });

  it('falls back to dark for anything else', () => {
    expect(parseTheme(null)).toBe('dark');
    expect(parseTheme('')).toBe('dark');
    expect(parseTheme('solarized')).toBe('dark');
  });
});

describe('a preference that outlives the tab', () => {
  it('round-trips through storage', () => {
    writeTheme('light');
    expect(window.localStorage.getItem(THEME_KEY)).toBe('light');
    expect(readTheme()).toBe('light');
  });

  it('defaults to dark when nothing was stored', () => {
    expect(readTheme()).toBe('dark');
  });

  it('falls back to dark when storage refuses to be read', () => {
    vi.spyOn(window.localStorage, 'getItem').mockImplementation(() => {
      throw new Error('denied');
    });

    expect(readTheme()).toBe('dark');
  });

  it('gives up quietly when storage refuses to be written', () => {
    vi.spyOn(window.localStorage, 'setItem').mockImplementation(() => {
      throw new Error('denied');
    });

    expect(() => {
      writeTheme('light');
    }).not.toThrow();
  });
});

describe('systemTheme', () => {
  it('follows the media query both ways', () => {
    setSystemDark(true);
    expect(systemTheme()).toBe('dark');

    setSystemDark(false);
    expect(systemTheme()).toBe('light');
  });
});

describe('watchSystemTheme', () => {
  it('reports a change in either direction', () => {
    const seen: string[] = [];
    watchSystemTheme((theme) => {
      seen.push(theme);
    });

    emitSystemDark(true);
    emitSystemDark(false);

    expect(seen).toEqual(['dark', 'light']);
  });
});

describe('resolveTheme', () => {
  it('takes an explicit preference at its word', () => {
    expect(resolveTheme('dark', 'light')).toBe('dark');
    expect(resolveTheme('light', 'dark')).toBe('light');
  });

  it('defers to the system only when asked to', () => {
    expect(resolveTheme('system', 'dark')).toBe('dark');
    expect(resolveTheme('system', 'light')).toBe('light');
  });
});

describe('applyTheme', () => {
  it('stamps the resolved theme on the document', () => {
    applyTheme('light');
    expect(document.documentElement.dataset.theme).toBe('light');

    applyTheme('dark');
    expect(document.documentElement.dataset.theme).toBe('dark');
  });
});

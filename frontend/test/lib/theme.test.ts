import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  BUILT_IN_THEMES,
  applyTheme,
  parseTheme,
  readTheme,
  resolveTheme,
  themeById,
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
    expect(resolveTheme(BUILT_IN_THEMES, 'dark', 'light').id).toBe('dark');
    expect(resolveTheme(BUILT_IN_THEMES, 'light', 'dark').id).toBe('light');
  });

  it('defers to the system only when asked to', () => {
    expect(resolveTheme(BUILT_IN_THEMES, 'system', 'dark').id).toBe('dark');
    expect(resolveTheme(BUILT_IN_THEMES, 'system', 'light').id).toBe('light');
  });

  it('falls back to dark for a theme that is no longer installed', () => {
    expect(themeById(BUILT_IN_THEMES, 'solarized').id).toBe('dark');
  });
});

describe('applyTheme', () => {
  it('stamps the base on the document', () => {
    applyTheme(themeById(BUILT_IN_THEMES, 'light'));
    expect(document.documentElement.dataset.theme).toBe('light');

    applyTheme(themeById(BUILT_IN_THEMES, 'dark'));
    expect(document.documentElement.dataset.theme).toBe('dark');
  });

  it('writes the tokens of a custom theme as inline variables over its base', () => {
    applyTheme({ id: 'custom', name: 'Custom', base: 'light', tokens: { surface: '#fdf6e3' } });

    expect(document.documentElement.dataset.theme).toBe('light');
    expect(document.documentElement.style.getPropertyValue('--surface')).toBe('#fdf6e3');
  });

  it('takes the variables of the previous theme back off', () => {
    applyTheme({ id: 'custom', name: 'Custom', base: 'light', tokens: { surface: '#fdf6e3' } });
    applyTheme(themeById(BUILT_IN_THEMES, 'dark'));

    expect(document.documentElement.style.getPropertyValue('--surface')).toBe('');
  });
});

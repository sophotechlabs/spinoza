import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  BLADE_RUNNER,
  BUILT_IN_THEMES,
  CANVAS_NAMES,
  CYBERPUNK,
  MATRIX,
  NORD,
  PAINTED_KEY,
  STARTREKTOR,
  TOKEN_NAMES,
  applyTheme,
  painted,
  parseTheme,
  readTheme,
  resolveTheme,
  themeById,
  systemTheme,
  watchSystemTheme,
  writeTheme,
  THEME_KEY,
  SURFACE_TOKENS,
  backgroundsFor,
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

  it('falls back to dark when nothing was stored', () => {
    expect(parseTheme(null)).toBe('dark');
    expect(parseTheme('')).toBe('dark');
  });

  it('passes a custom id through for the registry to resolve', () => {
    expect(parseTheme('solarized')).toBe('solarized');
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

describe('which backgrounds a token has to clear', () => {
  it('holds ordinary text to every surface it can land on', () => {
    expect(backgroundsFor('fg-subtle')).toEqual(SURFACE_TOKENS);
    expect(backgroundsFor('ok')).toEqual(SURFACE_TOKENS);
  });

  it('holds tint-only text to its own tint', () => {
    expect(backgroundsFor('ok-contrast')).toEqual(['ok-tint']);
    expect(backgroundsFor('error-strong')).toEqual(['error-tint']);
    expect(backgroundsFor('warn-strong')).toEqual(['warn-tint']);
    expect(backgroundsFor('info-contrast')).toEqual(['info-tint']);
  });

  it('holds terminal colours to the terminal background', () => {
    expect(backgroundsFor('ansi-red')).toEqual(['surface']);
    expect(backgroundsFor('ansi-bright-white')).toEqual(['surface']);
  });
});

describe('the themes that ship with spinoza', () => {
  it('are offered next to the two plain ones', () => {
    expect(BUILT_IN_THEMES.map((theme) => theme.id)).toEqual([
      'dark',
      'light',
      'nord',
      'blade-runner',
      'cyberpunk',
      'matrix',
      'startrektor',
    ]);
  });

  for (const theme of [NORD, BLADE_RUNNER, CYBERPUNK, MATRIX, STARTREKTOR]) {
    it(`${theme.name} sets every token and every canvas colour, so nothing falls back to dark`, () => {
      expect(Object.keys(theme.tokens ?? {}).sort()).toEqual([...TOKEN_NAMES].sort());
      expect(Object.keys(theme.canvas ?? {}).sort()).toEqual([...CANVAS_NAMES].sort());
    });

    it(`${theme.name} builds on the dark base, so the editor and graph follow it`, () => {
      expect(theme.base).toBe('dark');
    });
  }

  it('reaches the DOM as inline custom properties', () => {
    applyTheme(NORD);

    expect(document.documentElement.dataset.theme).toBe('dark');
    expect(document.documentElement.style.getPropertyValue('--surface')).toBe('#2e3440');
  });
});

describe('what the pre-paint script replays', () => {
  it('records the base and tokens actually applied', () => {
    applyTheme(NORD);

    const stored: unknown = JSON.parse(window.localStorage.getItem(PAINTED_KEY) ?? 'null');
    expect(stored).toEqual({ base: 'dark', tokens: NORD.tokens });
  });

  it('records an empty token map for a theme whose colours live in the CSS', () => {
    applyTheme(BUILT_IN_THEMES[1]);

    expect(painted(BUILT_IN_THEMES[1])).toEqual({ base: 'light', tokens: {} });
  });

  it('survives storage it cannot write to', () => {
    const setItem = vi.spyOn(window.localStorage, 'setItem').mockImplementation(() => {
      throw new Error('quota');
    });

    expect(() => {
      applyTheme(NORD);
    }).not.toThrow();
    setItem.mockRestore();
  });
});

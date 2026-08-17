import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { writeStored } from '../../src/lib/persist';
import { CONTENT_TOKENS, SURFACE_TOKENS } from '../../src/lib/theme';
import {
  CUSTOM_THEMES_KEY,
  isColor,
  parseCustomThemes,
  readCustomThemes,
  validateTheme,
  writeCustomThemes,
} from '../../src/lib/customThemes';

const SOLARIZED = {
  id: 'solarized',
  name: 'Solarized Light',
  base: 'light',
  tokens: { surface: '#fdf6e3', fg: 'oklch(50% 0.02 200)' },
  canvas: { terminalBackground: '#fdf6e3' },
};

afterEach(() => {
  vi.restoreAllMocks();
  window.localStorage.clear();
});

describe('isColor', () => {
  it('accepts the notations a person would actually write', () => {
    expect(isColor('#fff')).toBe(true);
    expect(isColor('#fdf6e3')).toBe(true);
    expect(isColor('#fdf6e380')).toBe(true);
    expect(isColor('rgb(1, 2, 3)')).toBe(true);
    expect(isColor('oklch(50% 0.02 200)')).toBe(true);
    expect(isColor('  hsl(200 50% 50%)  ')).toBe(true);
  });

  it('rejects anything it cannot be sure about', () => {
    expect(isColor('rebeccapurple')).toBe(false);
    expect(isColor('#ff')).toBe(false);
    expect(isColor('rgb()')).toBe(false);
    expect(isColor('javascript:alert(1)')).toBe(false);
    expect(isColor('')).toBe(false);
  });
});

describe('a theme someone hands us', () => {
  it('is accepted when it is well formed', () => {
    const { theme, errors } = validateTheme(SOLARIZED);

    expect(errors).toEqual([]);
    expect(theme?.id).toBe('solarized');
    expect(theme?.base).toBe('light');
    expect(theme?.tokens?.surface).toBe('#fdf6e3');
    expect(theme?.canvas?.terminalBackground).toBe('#fdf6e3');
  });

  it('needs an id, a name and a base', () => {
    const { theme, errors } = validateTheme({});

    expect(theme).toBeNull();
    expect(errors).toContain('id is required');
    expect(errors).toContain('name is required');
    expect(errors).toContain('base must be "dark" or "light"');
  });

  it('refuses to shadow a built-in', () => {
    const { errors } = validateTheme({ ...SOLARIZED, id: 'dark' });

    expect(errors).toContain('id "dark" is reserved for a built-in theme');
  });

  it('rejects a token name it does not know, rather than ignoring it', () => {
    const { theme, errors } = validateTheme({
      ...SOLARIZED,
      tokens: { 'not-a-token': '#fdf6e3' },
    });

    expect(theme).toBeNull();
    expect(errors).toContain('tokens: "not-a-token" is not a known name');
  });

  it('rejects a value that is not a colour', () => {
    const { errors } = validateTheme({ ...SOLARIZED, tokens: { surface: 'beige' } });

    expect(errors).toContain('tokens: "surface" is not a colour this understands');
  });

  it('rejects an unknown canvas name too', () => {
    const { errors } = validateTheme({ ...SOLARIZED, canvas: { terminalBg: '#fff' } });

    expect(errors).toContain('canvas: "terminalBg" is not a known name');
  });

  it('rejects tokens that are not an object', () => {
    const { errors } = validateTheme({ ...SOLARIZED, tokens: [] });

    expect(errors).toContain('tokens must be an object');
  });

  it('rejects anything that is not an object at all', () => {
    expect(validateTheme('nope').errors).toEqual(['a theme must be a JSON object']);
    expect(validateTheme(null).errors).toEqual(['a theme must be a JSON object']);
  });

  it('says so when a theme leaves the background to its base', () => {
    const { theme, warnings } = validateTheme({
      id: 'accent',
      name: 'Accent',
      base: 'dark',
      tokens: { ok: '#00ff00' },
    });

    expect(theme).not.toBeNull();
    expect(warnings).toEqual([
      'this theme does not set surface, so it inherits the background of its base',
    ]);
  });

  it('keeps quiet about the background when the theme sets one', () => {
    expect(validateTheme(SOLARIZED).warnings).toEqual([]);
  });
});

describe('the themes a person has installed', () => {
  it('round-trip through storage', () => {
    const { theme } = validateTheme(SOLARIZED);
    writeCustomThemes([theme as never]);

    expect(readCustomThemes()).toHaveLength(1);
    expect(readCustomThemes()[0].id).toBe('solarized');
  });

  it('start empty when nothing is stored', () => {
    expect(readCustomThemes()).toEqual([]);
  });

  it('drop an entry that no longer validates instead of failing to load', () => {
    writeStored(CUSTOM_THEMES_KEY, JSON.stringify([SOLARIZED, { id: 'broken' }]));

    expect(readCustomThemes()).toHaveLength(1);
  });

  it('survive junk in storage', () => {
    expect(parseCustomThemes(null)).toEqual([]);
    expect(parseCustomThemes('not json')).toEqual([]);
    expect(parseCustomThemes('{"not":"an array"}')).toEqual([]);
  });

  it('survive storage that refuses to be read or written', () => {
    vi.spyOn(window.localStorage, 'getItem').mockImplementation(() => {
      throw new Error('denied');
    });
    expect(readCustomThemes()).toEqual([]);

    vi.spyOn(window.localStorage, 'setItem').mockImplementation(() => {
      throw new Error('denied');
    });
    expect(() => {
      writeCustomThemes([]);
    }).not.toThrow();
  });
});

describe('a theme that would be hard to read', () => {
  it('imports but says which colours fall below AA', () => {
    const { theme, warnings } = validateTheme({
      id: 'faint',
      name: 'Faint',
      base: 'light',
      tokens: { surface: '#ffffff', fg: '#eeeeee' },
    });

    expect(theme).not.toBeNull();
    expect(warnings.some((line) => line.startsWith('fg is'))).toBe(true);
  });

  it('stays quiet for a theme that is legible', () => {
    const { warnings } = validateTheme({
      id: 'fine',
      name: 'Fine',
      base: 'light',
      tokens: { surface: '#ffffff', fg: '#111111' },
    });

    expect(warnings).toEqual([]);
  });
});

describe('an imported theme measured against the base it sits on', () => {
  beforeEach(() => {
    const root = document.documentElement;
    for (const name of SURFACE_TOKENS) {
      root.style.setProperty(`--${name}`, '#ffffff');
    }
    for (const token of CONTENT_TOKENS) {
      root.style.setProperty(`--${token}`, '#111111');
    }
    for (const tint of ['ok-tint', 'error-tint', 'warn-tint', 'info-tint']) {
      root.style.setProperty(`--${tint}`, '#ffffff');
    }
  });

  afterEach(() => {
    document.documentElement.removeAttribute('style');
  });

  it('warns when an override is unreadable on a background it never set', () => {
    const check = validateTheme({
      id: 'washed',
      name: 'Washed',
      base: 'light',
      tokens: { fg: '#f2f2f2' },
    });

    expect(check.theme).not.toBeNull();
    expect(check.warnings.some((line) => line.startsWith('fg is'))).toBe(true);
  });

  it('warns when a background override makes the base text unreadable', () => {
    const check = validateTheme({
      id: 'muddy',
      name: 'Muddy',
      base: 'light',
      tokens: { 'surface-active': '#3a3a3a' },
    });

    expect(check.warnings.some((line) => line.includes('on surface-active'))).toBe(true);
  });
});

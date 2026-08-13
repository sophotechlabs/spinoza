import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { BUILT_IN_THEMES, THEME_KEY } from '../../src/lib/theme';
import { emitSystemDark, setSystemDark } from '../helpers';

async function freshStore() {
  vi.resetModules();
  return import('../../src/store/theme');
}

beforeEach(() => {
  window.localStorage.clear();
  setSystemDark(false);
  delete document.documentElement.dataset.theme;
});

afterEach(() => {
  window.localStorage.clear();
  setSystemDark(false);
});

describe('the theme a returning user gets', () => {
  it('is dark when nothing was ever chosen', async () => {
    const { useThemeStore } = await freshStore();

    expect(useThemeStore.getState().preference).toBe('dark');
    expect(useThemeStore.getState().resolved.id).toBe('dark');
    expect(document.documentElement.dataset.theme).toBe('dark');
  });

  it('is whatever they picked last time', async () => {
    window.localStorage.setItem(THEME_KEY, 'light');
    const { useThemeStore } = await freshStore();

    expect(useThemeStore.getState().preference).toBe('light');
    expect(useThemeStore.getState().resolved.id).toBe('light');
    expect(document.documentElement.dataset.theme).toBe('light');
  });

  it('follows the operating system when they asked it to', async () => {
    window.localStorage.setItem(THEME_KEY, 'system');
    setSystemDark(true);
    const { useThemeStore } = await freshStore();

    expect(useThemeStore.getState().preference).toBe('system');
    expect(useThemeStore.getState().resolved.id).toBe('dark');
  });
});

describe('choosing a theme', () => {
  it('applies it, remembers it and resolves it', async () => {
    const { useThemeStore } = await freshStore();

    useThemeStore.getState().setPreference('light');

    expect(useThemeStore.getState().resolved.id).toBe('light');
    expect(window.localStorage.getItem(THEME_KEY)).toBe('light');
    expect(document.documentElement.dataset.theme).toBe('light');
  });

  it('resolves system against what the system currently says', async () => {
    setSystemDark(true);
    const { useThemeStore } = await freshStore();

    useThemeStore.getState().setPreference('system');

    expect(useThemeStore.getState().resolved.id).toBe('dark');
  });
});

describe('the operating system changing under us', () => {
  it('repaints while the preference is system', async () => {
    window.localStorage.setItem(THEME_KEY, 'system');
    const { useThemeStore } = await freshStore();

    expect(useThemeStore.getState().resolved.id).toBe('light');

    emitSystemDark(true);
    expect(useThemeStore.getState().resolved.id).toBe('dark');
    expect(document.documentElement.dataset.theme).toBe('dark');

    emitSystemDark(false);
    expect(useThemeStore.getState().resolved.id).toBe('light');
  });

  it('is remembered but ignored while an explicit theme is set', async () => {
    window.localStorage.setItem(THEME_KEY, 'light');
    const { useThemeStore } = await freshStore();

    emitSystemDark(true);

    expect(useThemeStore.getState().system).toBe('dark');
    expect(useThemeStore.getState().resolved.id).toBe('light');
    expect(document.documentElement.dataset.theme).toBe('light');
  });
});

describe('installing a theme someone imported', () => {
  const solarized = {
    id: 'solarized',
    name: 'Solarized',
    base: 'light' as const,
    tokens: { surface: '#fdf6e3' },
  };

  it('replaces an earlier one with the same id instead of stacking duplicates', async () => {
    const { useThemeStore } = await freshStore();

    useThemeStore.getState().addTheme(solarized);
    useThemeStore.getState().addTheme({ ...solarized, name: 'Solarized Light' });

    expect(useThemeStore.getState().custom).toHaveLength(1);
    expect(useThemeStore.getState().custom[0].name).toBe('Solarized Light');
    expect(useThemeStore.getState().themes).toHaveLength(BUILT_IN_THEMES.length + 1);
  });

  it('drops back to a built-in when the selected theme is removed', async () => {
    const { useThemeStore } = await freshStore();
    useThemeStore.getState().addTheme(solarized);
    useThemeStore.getState().setPreference('solarized');
    expect(useThemeStore.getState().resolved.id).toBe('solarized');

    useThemeStore.getState().removeTheme('solarized');

    expect(useThemeStore.getState().resolved.id).toBe('dark');
    expect(document.documentElement.style.getPropertyValue('--surface')).toBe('');
  });
});

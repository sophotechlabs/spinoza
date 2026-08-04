import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { THEME_KEY } from '../../src/lib/theme';
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
    expect(useThemeStore.getState().resolved).toBe('dark');
    expect(document.documentElement.dataset.theme).toBe('dark');
  });

  it('is whatever they picked last time', async () => {
    window.localStorage.setItem(THEME_KEY, 'light');
    const { useThemeStore } = await freshStore();

    expect(useThemeStore.getState().preference).toBe('light');
    expect(useThemeStore.getState().resolved).toBe('light');
    expect(document.documentElement.dataset.theme).toBe('light');
  });

  it('follows the operating system when they asked it to', async () => {
    window.localStorage.setItem(THEME_KEY, 'system');
    setSystemDark(true);
    const { useThemeStore } = await freshStore();

    expect(useThemeStore.getState().preference).toBe('system');
    expect(useThemeStore.getState().resolved).toBe('dark');
  });
});

describe('choosing a theme', () => {
  it('applies it, remembers it and resolves it', async () => {
    const { useThemeStore } = await freshStore();

    useThemeStore.getState().setPreference('light');

    expect(useThemeStore.getState().resolved).toBe('light');
    expect(window.localStorage.getItem(THEME_KEY)).toBe('light');
    expect(document.documentElement.dataset.theme).toBe('light');
  });

  it('resolves system against what the system currently says', async () => {
    setSystemDark(true);
    const { useThemeStore } = await freshStore();

    useThemeStore.getState().setPreference('system');

    expect(useThemeStore.getState().resolved).toBe('dark');
  });
});

describe('the operating system changing under us', () => {
  it('repaints while the preference is system', async () => {
    window.localStorage.setItem(THEME_KEY, 'system');
    const { useThemeStore } = await freshStore();

    expect(useThemeStore.getState().resolved).toBe('light');

    emitSystemDark(true);
    expect(useThemeStore.getState().resolved).toBe('dark');
    expect(document.documentElement.dataset.theme).toBe('dark');

    emitSystemDark(false);
    expect(useThemeStore.getState().resolved).toBe('light');
  });

  it('is remembered but ignored while an explicit theme is set', async () => {
    window.localStorage.setItem(THEME_KEY, 'light');
    const { useThemeStore } = await freshStore();

    emitSystemDark(true);

    expect(useThemeStore.getState().system).toBe('dark');
    expect(useThemeStore.getState().resolved).toBe('light');
    expect(document.documentElement.dataset.theme).toBe('light');
  });
});

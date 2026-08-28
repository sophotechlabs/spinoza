import { create } from 'zustand';
import type { Theme, ThemeBase, ThemePreference } from '../lib/theme';
import {
  BUILT_IN_THEMES,
  applyTheme,
  readTheme,
  resolveTheme,
  systemTheme,
  watchSystemTheme,
  writeTheme,
} from '../lib/theme';
import { readCustomThemes, writeCustomThemes } from '../lib/customThemes';

interface ThemeState {
  themes: Theme[];
  custom: Theme[];
  preference: ThemePreference;
  system: ThemeBase;
  resolved: Theme;
  setPreference: (preference: ThemePreference) => void;
  setSystem: (system: ThemeBase) => void;
  addTheme: (theme: Theme) => void;
  removeTheme: (id: string) => void;
  adoptStored: () => void;
}

const preference = readTheme();
const system = systemTheme();
const custom = readCustomThemes();
const themes = [...BUILT_IN_THEMES, ...custom];

export const useThemeStore = create<ThemeState>((set, get) => ({
  themes,
  custom,
  preference,
  system,
  resolved: resolveTheme(themes, preference, system),
  setPreference: (next) => {
    const resolved = resolveTheme(get().themes, next, get().system);
    writeTheme(next);
    applyTheme(resolved);
    set({ preference: next, resolved });
  },
  setSystem: (next) => {
    const resolved = resolveTheme(get().themes, get().preference, next);
    applyTheme(resolved);
    set({ system: next, resolved });
  },
  addTheme: (theme) => {
    const kept = get().custom.filter((entry) => entry.id !== theme.id);
    const nextCustom = [...kept, theme];
    const nextThemes = [...BUILT_IN_THEMES, ...nextCustom];
    writeCustomThemes(nextCustom);
    const resolved = resolveTheme(nextThemes, get().preference, get().system);
    applyTheme(resolved);
    set({ custom: nextCustom, themes: nextThemes, resolved });
  },
  // adoptStored takes what is in the settings now, without writing it back. It
  // is for a change another window made, which is already saved.
  adoptStored: () => {
    const nextPreference = readTheme();
    const nextCustom = readCustomThemes();
    const nextThemes = [...BUILT_IN_THEMES, ...nextCustom];
    const resolved = resolveTheme(nextThemes, nextPreference, get().system);
    applyTheme(resolved);
    set({ preference: nextPreference, custom: nextCustom, themes: nextThemes, resolved });
  },
  removeTheme: (id) => {
    const nextCustom = get().custom.filter((entry) => entry.id !== id);
    const nextThemes = [...BUILT_IN_THEMES, ...nextCustom];
    writeCustomThemes(nextCustom);
    const resolved = resolveTheme(nextThemes, get().preference, get().system);
    applyTheme(resolved);
    set({ custom: nextCustom, themes: nextThemes, resolved });
  },
}));

applyTheme(useThemeStore.getState().resolved);

watchSystemTheme((next) => {
  useThemeStore.getState().setSystem(next);
});

export function useResolvedTheme(): Theme {
  return useThemeStore((state) => state.resolved);
}

export function useThemePreference(): ThemePreference {
  return useThemeStore((state) => state.preference);
}

export function useThemes(): Theme[] {
  return useThemeStore((state) => state.themes);
}

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

interface ThemeState {
  themes: Theme[];
  preference: ThemePreference;
  system: ThemeBase;
  resolved: Theme;
  setPreference: (preference: ThemePreference) => void;
  setSystem: (system: ThemeBase) => void;
}

const preference = readTheme();
const system = systemTheme();

export const useThemeStore = create<ThemeState>((set, get) => ({
  themes: BUILT_IN_THEMES,
  preference,
  system,
  resolved: resolveTheme(BUILT_IN_THEMES, preference, system),
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

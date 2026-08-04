import { create } from 'zustand';
import type { ResolvedTheme, ThemePreference } from '../lib/theme';
import {
  applyTheme,
  readTheme,
  resolveTheme,
  systemTheme,
  watchSystemTheme,
  writeTheme,
} from '../lib/theme';

interface ThemeState {
  preference: ThemePreference;
  system: ResolvedTheme;
  resolved: ResolvedTheme;
  setPreference: (preference: ThemePreference) => void;
  setSystem: (system: ResolvedTheme) => void;
}

const preference = readTheme();
const system = systemTheme();

export const useThemeStore = create<ThemeState>((set, get) => ({
  preference,
  system,
  resolved: resolveTheme(preference, system),
  setPreference: (next) => {
    const resolved = resolveTheme(next, get().system);
    writeTheme(next);
    applyTheme(resolved);
    set({ preference: next, resolved });
  },
  setSystem: (next) => {
    const resolved = resolveTheme(get().preference, next);
    applyTheme(resolved);
    set({ system: next, resolved });
  },
}));

applyTheme(useThemeStore.getState().resolved);

watchSystemTheme((next) => {
  useThemeStore.getState().setSystem(next);
});

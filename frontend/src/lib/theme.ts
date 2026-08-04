export const THEMES = ['dark', 'light', 'system'] as const;

export type ThemePreference = (typeof THEMES)[number];

export type ThemeBase = 'dark' | 'light';

export interface CanvasColors {
  chartAxis: string;
  chartGrid: string;
  cpuStroke: string;
  cpuFill: string;
  memoryStroke: string;
  memoryFill: string;
  terminalBackground: string;
  terminalForeground: string;
}

export interface Theme {
  id: string;
  name: string;
  base: ThemeBase;
  tokens?: Record<string, string>;
  canvas?: Partial<CanvasColors>;
}

export const BUILT_IN_THEMES: Theme[] = [
  { id: 'dark', name: 'Dark', base: 'dark' },
  { id: 'light', name: 'Light', base: 'light' },
];

export const THEME_KEY = 'spinoza.theme.v1';

const DARK_QUERY = '(prefers-color-scheme: dark)';

export function parseTheme(raw: string | null): ThemePreference {
  for (const theme of THEMES) {
    if (theme === raw) {
      return theme;
    }
  }
  return 'dark';
}

export function readTheme(): ThemePreference {
  try {
    return parseTheme(window.localStorage.getItem(THEME_KEY));
  } catch {
    return 'dark';
  }
}

export function writeTheme(preference: ThemePreference): void {
  try {
    window.localStorage.setItem(THEME_KEY, preference);
  } catch {
    return;
  }
}

export function systemTheme(): ThemeBase {
  if (window.matchMedia(DARK_QUERY).matches) {
    return 'dark';
  }
  return 'light';
}

export function watchSystemTheme(onChange: (base: ThemeBase) => void): void {
  window.matchMedia(DARK_QUERY).addEventListener('change', (event) => {
    if (event.matches) {
      onChange('dark');
      return;
    }
    onChange('light');
  });
}

export function themeById(themes: Theme[], id: string): Theme {
  for (const theme of themes) {
    if (theme.id === id) {
      return theme;
    }
  }
  return BUILT_IN_THEMES[0];
}

export function resolveTheme(
  themes: Theme[],
  preference: ThemePreference,
  system: ThemeBase,
): Theme {
  if (preference === 'system') {
    return themeById(themes, system);
  }
  return themeById(themes, preference);
}

let applied: string[] = [];

export function applyTheme(theme: Theme): void {
  const root = document.documentElement;
  for (const name of applied) {
    root.style.removeProperty(name);
  }
  applied = [];

  root.dataset.theme = theme.base;
  root.style.colorScheme = theme.base;

  if (theme.tokens === undefined) {
    return;
  }
  for (const [token, value] of Object.entries(theme.tokens)) {
    const name = `--${token}`;
    root.style.setProperty(name, value);
    applied.push(name);
  }
}

export const SYSTEM = 'system';

export type ThemePreference = string;

export type ThemeBase = 'dark' | 'light';

export const TOKEN_NAMES = [
  'surface',
  'surface-raised',
  'surface-active',
  'handle',
  'handle-active',
  'grip',
  'fg-strong',
  'fg',
  'fg-soft',
  'fg-muted',
  'fg-subtle',
  'fg-faint',
  'edge',
  'edge-strong',
  'edge-emphasis',
  'edge-active',
  'idle-solid',
  'ok',
  'ok-contrast',
  'ok-solid',
  'ok-tint',
  'ok-emphasis',
  'ok-line',
  'ok-line-strong',
  'error',
  'error-strong',
  'error-contrast',
  'error-muted',
  'error-solid',
  'error-tint',
  'error-tint-strong',
  'error-emphasis',
  'error-line',
  'error-line-strong',
  'warn',
  'warn-strong',
  'warn-muted',
  'warn-solid',
  'warn-tint',
  'warn-line',
  'warn-line-strong',
  'info-contrast',
  'info-tint',
  'info-line',
  'ansi-black',
  'ansi-red',
  'ansi-green',
  'ansi-yellow',
  'ansi-blue',
  'ansi-magenta',
  'ansi-cyan',
  'ansi-white',
  'ansi-bright-black',
  'ansi-bright-red',
  'ansi-bright-green',
  'ansi-bright-yellow',
  'ansi-bright-blue',
  'ansi-bright-magenta',
  'ansi-bright-cyan',
  'ansi-bright-white',
];

export const CONTENT_TOKENS = [
  'fg-strong',
  'fg',
  'fg-soft',
  'fg-muted',
  'fg-subtle',
  'ok',
  'ok-contrast',
  'error',
  'error-strong',
  'error-contrast',
  'warn',
  'warn-strong',
  'warn-muted',
  'info-contrast',
  'ansi-black',
  'ansi-red',
  'ansi-green',
  'ansi-yellow',
  'ansi-blue',
  'ansi-magenta',
  'ansi-cyan',
  'ansi-white',
  'ansi-bright-black',
  'ansi-bright-red',
  'ansi-bright-green',
  'ansi-bright-yellow',
  'ansi-bright-blue',
  'ansi-bright-magenta',
  'ansi-bright-cyan',
  'ansi-bright-white',
];

export const SURFACE_TOKENS = ['surface', 'surface-raised', 'surface-active'];

export const TINT_BACKGROUNDS: Partial<Record<string, string>> = {
  'ok-contrast': 'ok-tint',
  'error-strong': 'error-tint',
  'error-contrast': 'error-tint',
  'error-muted': 'error-tint',
  'warn-strong': 'warn-tint',
  'info-contrast': 'info-tint',
};

export function backgroundsFor(token: string): string[] {
  const tint = TINT_BACKGROUNDS[token];
  if (tint !== undefined) {
    return [tint];
  }
  if (token.startsWith('ansi-')) {
    return ['surface'];
  }
  return SURFACE_TOKENS;
}

export const CANVAS_NAMES = [
  'chartAxis',
  'chartGrid',
  'cpuStroke',
  'cpuFill',
  'memoryStroke',
  'memoryFill',
  'terminalBackground',
  'terminalForeground',
];

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
  if (raw === null || raw === '') {
    return 'dark';
  }
  return raw;
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
  if (preference === SYSTEM) {
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

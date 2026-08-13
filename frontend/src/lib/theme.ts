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

export const NORD: Theme = {
  id: 'nord',
  name: 'Nord',
  base: 'dark',
  tokens: {
    surface: '#2e3440',
    'surface-raised': '#3b4252',
    'surface-active': '#4c566a',
    handle: '#4c566a',
    'handle-active': '#88c0d0',
    grip: '#434c5e',
    'fg-strong': '#eceff4',
    fg: '#e5e9f0',
    'fg-soft': '#d8dee9',
    'fg-muted': '#b8c3d4',
    'fg-subtle': '#9aa6bd',
    'fg-faint': '#6d7a91',
    edge: '#3b4252',
    'edge-strong': '#4c566a',
    'edge-emphasis': '#5b6a86',
    'edge-active': '#88c0d0',
    'idle-solid': '#6d7a91',
    ok: '#a3be8c',
    'ok-contrast': '#d5e3c6',
    'ok-solid': '#a3be8c',
    'ok-tint': '#3a4536',
    'ok-emphasis': '#8faa78',
    'ok-line': '#4f6047',
    'ok-line-strong': '#6b8060',
    error: '#d08c92',
    'error-strong': '#d28c76',
    'error-contrast': '#e8c3c6',
    'error-muted': '#a54e56',
    'error-solid': '#bf616a',
    'error-tint': '#432c30',
    'error-tint-strong': '#5a3a3f',
    'error-emphasis': '#a54e56',
    'error-line': '#5a3a3f',
    'error-line-strong': '#7a4d54',
    warn: '#ebcb8b',
    'warn-strong': '#f0d9a8',
    'warn-muted': '#d0b06a',
    'warn-solid': '#ebcb8b',
    'warn-tint': '#463c26',
    'warn-line': '#5e5133',
    'warn-line-strong': '#7d6c45',
    'info-contrast': '#c8dbe4',
    'info-tint': '#2b3b45',
    'info-line': '#4a707f',
    'ansi-black': '#949fb3',
    'ansi-red': '#d08c92',
    'ansi-green': '#a3be8c',
    'ansi-yellow': '#ebcb8b',
    'ansi-blue': '#81a1c1',
    'ansi-magenta': '#b793b0',
    'ansi-cyan': '#88c0d0',
    'ansi-white': '#e5e9f0',
    'ansi-bright-black': '#929eb6',
    'ansi-bright-red': '#d28c76',
    'ansi-bright-green': '#b5cd9f',
    'ansi-bright-yellow': '#f0d9a8',
    'ansi-bright-blue': '#8fbcbb',
    'ansi-bright-magenta': '#c9a2c2',
    'ansi-bright-cyan': '#a3d3e0',
    'ansi-bright-white': '#eceff4',
  },
  canvas: {
    chartAxis: '#9aa6bd',
    chartGrid: '#3b4252',
    cpuStroke: '#a3be8c',
    cpuFill: 'rgba(163,190,140,0.14)',
    memoryStroke: '#81a1c1',
    memoryFill: 'rgba(129,161,193,0.14)',
    terminalBackground: '#2e3440',
    terminalForeground: '#d8dee9',
  },
};

export const BUILT_IN_THEMES: Theme[] = [
  { id: 'dark', name: 'Dark', base: 'dark' },
  { id: 'light', name: 'Light', base: 'light' },
  NORD,
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

export const PAINTED_KEY = 'spinoza.painted.v1';

export interface PaintedTheme {
  base: ThemeBase;
  tokens: Record<string, string>;
}

export function painted(theme: Theme): PaintedTheme {
  return { base: theme.base, tokens: theme.tokens ?? {} };
}

function recordPainted(theme: Theme): void {
  try {
    window.localStorage.setItem(PAINTED_KEY, JSON.stringify(painted(theme)));
  } catch {
    return;
  }
}

export function applyTheme(theme: Theme): void {
  const root = document.documentElement;
  for (const name of applied) {
    root.style.removeProperty(name);
  }
  applied = [];

  root.dataset.theme = theme.base;
  root.style.colorScheme = theme.base;
  recordPainted(theme);

  if (theme.tokens === undefined) {
    return;
  }
  for (const [token, value] of Object.entries(theme.tokens)) {
    const name = `--${token}`;
    root.style.setProperty(name, value);
    applied.push(name);
  }
}

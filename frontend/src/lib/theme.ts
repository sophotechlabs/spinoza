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

export const BLADE_RUNNER: Theme = {
  id: 'blade-runner',
  name: 'Blade Runner',
  base: 'dark',
  tokens: {
    surface: '#050b0d',
    'surface-raised': '#081418',
    'surface-active': '#0d222a',
    handle: '#164450',
    'handle-active': '#ffb454',
    grip: '#103038',
    'fg-strong': '#ffd9a0',
    fg: '#ffb454',
    'fg-soft': '#f09c3a',
    'fg-muted': '#d98a28',
    'fg-subtle': '#c47c1e',
    'fg-faint': '#705626',
    edge: '#103038',
    'edge-strong': '#164450',
    'edge-emphasis': '#1d5a6a',
    'edge-active': '#ffb454',
    'idle-solid': '#4d7a88',
    ok: '#43d9ad',
    'ok-contrast': '#b5f0dd',
    'ok-solid': '#2eb890',
    'ok-tint': '#06291f',
    'ok-emphasis': '#219478',
    'ok-line': '#0f4536',
    'ok-line-strong': '#14614b',
    error: '#ff5f6e',
    'error-strong': '#ff8d98',
    'error-contrast': '#ffc2c8',
    'error-muted': '#ff7581',
    'error-solid': '#e5384a',
    'error-tint': '#330a12',
    'error-tint-strong': '#4d111f',
    'error-emphasis': '#cc2e40',
    'error-line': '#4d111f',
    'error-line-strong': '#6b1a2b',
    warn: '#ff7f2e',
    'warn-strong': '#ffa366',
    'warn-muted': '#e06a1a',
    'warn-solid': '#f26f1c',
    'warn-tint': '#33170a',
    'warn-line': '#5c2c10',
    'warn-line-strong': '#7a3c15',
    'info-contrast': '#a8e8f5',
    'info-tint': '#082830',
    'info-line': '#14566a',
    'ansi-black': '#5d8a98',
    'ansi-red': '#ff5f6e',
    'ansi-green': '#43d9ad',
    'ansi-yellow': '#ffb454',
    'ansi-blue': '#57b0e8',
    'ansi-magenta': '#ff7eb6',
    'ansi-cyan': '#5fd7e8',
    'ansi-white': '#ecdcc0',
    'ansi-bright-black': '#6e9aa8',
    'ansi-bright-red': '#ff8d98',
    'ansi-bright-green': '#7de8c8',
    'ansi-bright-yellow': '#ffcc85',
    'ansi-bright-blue': '#85c8f0',
    'ansi-bright-magenta': '#ffa3cc',
    'ansi-bright-cyan': '#8ce3f0',
    'ansi-bright-white': '#fff2dc',
  },
  canvas: {
    chartAxis: '#c47c1e',
    chartGrid: '#0c242c',
    cpuStroke: '#ffb454',
    cpuFill: 'rgba(255,180,84,0.10)',
    memoryStroke: '#5fd7e8',
    memoryFill: 'rgba(95,215,232,0.10)',
    terminalBackground: '#030809',
    terminalForeground: '#ffb454',
  },
};

export const CYBERPUNK: Theme = {
  id: 'cyberpunk',
  name: 'Cyberpunk',
  base: 'dark',
  tokens: {
    surface: '#0b0b09',
    'surface-raised': '#13120c',
    'surface-active': '#1e1d10',
    handle: '#3d3a16',
    'handle-active': '#fcee0a',
    grip: '#2a2812',
    'fg-strong': '#fcee0a',
    fg: '#e8dc2e',
    'fg-soft': '#cfc528',
    'fg-muted': '#b0a81f',
    'fg-subtle': '#96901c',
    'fg-faint': '#55521a',
    edge: '#2a2812',
    'edge-strong': '#3d3a16',
    'edge-emphasis': '#55511c',
    'edge-active': '#fcee0a',
    'idle-solid': '#8a8560',
    ok: '#0aff9a',
    'ok-contrast': '#a8ffd9',
    'ok-solid': '#00e588',
    'ok-tint': '#03301d',
    'ok-emphasis': '#00c274',
    'ok-line': '#075534',
    'ok-line-strong': '#0a7347',
    error: '#ff4766',
    'error-strong': '#ff8296',
    'error-contrast': '#ffb8c4',
    'error-muted': '#ff5c73',
    'error-solid': '#ff003c',
    'error-tint': '#38050f',
    'error-tint-strong': '#52101f',
    'error-emphasis': '#d60031',
    'error-line': '#52101f',
    'error-line-strong': '#73172c',
    warn: '#ffa21e',
    'warn-strong': '#ffc266',
    'warn-muted': '#e08a10',
    'warn-solid': '#f59300',
    'warn-tint': '#2e1c05',
    'warn-line': '#5c3a0f',
    'warn-line-strong': '#7a4e12',
    'info-contrast': '#9df3ff',
    'info-tint': '#052e33',
    'info-line': '#0a626e',
    'ansi-black': '#85835f',
    'ansi-red': '#ff4766',
    'ansi-green': '#0aff9a',
    'ansi-yellow': '#fcee0a',
    'ansi-blue': '#3db8ff',
    'ansi-magenta': '#ff44a4',
    'ansi-cyan': '#5ef6ff',
    'ansi-white': '#e8e6c8',
    'ansi-bright-black': '#a3a17a',
    'ansi-bright-red': '#ff7a8f',
    'ansi-bright-green': '#66ffc2',
    'ansi-bright-yellow': '#fff566',
    'ansi-bright-blue': '#7dcfff',
    'ansi-bright-magenta': '#ff85c2',
    'ansi-bright-cyan': '#a3faff',
    'ansi-bright-white': '#fdf8d9',
  },
  canvas: {
    chartAxis: '#96901c',
    chartGrid: '#2a2812',
    cpuStroke: '#fcee0a',
    cpuFill: 'rgba(252,238,10,0.08)',
    memoryStroke: '#5ef6ff',
    memoryFill: 'rgba(94,246,255,0.10)',
    terminalBackground: '#060604',
    terminalForeground: '#fcee0a',
  },
};

export const MATRIX: Theme = {
  id: 'matrix',
  name: 'Matrix',
  base: 'dark',
  tokens: {
    surface: '#010502',
    'surface-raised': '#031008',
    'surface-active': '#062a15',
    handle: '#0c5c2a',
    'handle-active': '#00ff41',
    grip: '#08451f',
    'fg-strong': '#c2ffc2',
    fg: '#4dff66',
    'fg-soft': '#2ee854',
    'fg-muted': '#00d93c',
    'fg-subtle': '#00b030',
    'fg-faint': '#007a24',
    edge: '#052e14',
    'edge-strong': '#08451f',
    'edge-emphasis': '#0c5c2a',
    'edge-active': '#00ff41',
    'idle-solid': '#2e8a4a',
    ok: '#00ff41',
    'ok-contrast': '#b3ffc6',
    'ok-solid': '#00e639',
    'ok-tint': '#02290f',
    'ok-emphasis': '#00b82e',
    'ok-line': '#04511d',
    'ok-line-strong': '#067a2c',
    error: '#ff5140',
    'error-strong': '#ff9d8c',
    'error-contrast': '#ffc7bc',
    'error-muted': '#ff7a66',
    'error-solid': '#f0392e',
    'error-tint': '#2b0705',
    'error-tint-strong': '#47100a',
    'error-emphasis': '#cc2e24',
    'error-line': '#47100a',
    'error-line-strong': '#661a12',
    warn: '#ffb300',
    'warn-strong': '#ffcf66',
    'warn-muted': '#e09a00',
    'warn-solid': '#f5a800',
    'warn-tint': '#2e2103',
    'warn-line': '#55400a',
    'warn-line-strong': '#7a5c0e',
    'info-contrast': '#8affd6',
    'info-tint': '#013328',
    'info-line': '#02664f',
    'ansi-black': '#248f47',
    'ansi-red': '#ff5140',
    'ansi-green': '#00ff41',
    'ansi-yellow': '#ffb300',
    'ansi-blue': '#00d9a0',
    'ansi-magenta': '#9fffbe',
    'ansi-cyan': '#00ffd0',
    'ansi-white': '#b3ffc6',
    'ansi-bright-black': '#2ea057',
    'ansi-bright-red': '#ff7a66',
    'ansi-bright-green': '#66ff8a',
    'ansi-bright-yellow': '#ffcf66',
    'ansi-bright-blue': '#4dffc4',
    'ansi-bright-magenta': '#c2ffd6',
    'ansi-bright-cyan': '#66ffe0',
    'ansi-bright-white': '#e0ffe8',
  },
  canvas: {
    chartAxis: '#00b030',
    chartGrid: '#03260f',
    cpuStroke: '#00ff41',
    cpuFill: 'rgba(0,255,65,0.10)',
    memoryStroke: '#00e5c0',
    memoryFill: 'rgba(0,229,192,0.10)',
    terminalBackground: '#000000',
    terminalForeground: '#00ff41',
  },
};

export const STARTREKTOR: Theme = {
  id: 'startrektor',
  name: 'Startrektor',
  base: 'dark',
  tokens: {
    surface: '#000000',
    'surface-raised': '#0f0b06',
    'surface-active': '#241a0e',
    handle: '#8f5717',
    'handle-active': '#ff9900',
    grip: '#663f14',
    'fg-strong': '#ffddb3',
    fg: '#ffcc99',
    'fg-soft': '#ffb366',
    'fg-muted': '#ff9c66',
    'fg-subtle': '#e68a4d',
    'fg-faint': '#8a5c33',
    edge: '#663f14',
    'edge-strong': '#8f5717',
    'edge-emphasis': '#b8701c',
    'edge-active': '#ff9900',
    'idle-solid': '#aa88bb',
    ok: '#66d9a3',
    'ok-contrast': '#b8f0d8',
    'ok-solid': '#4dbf8a',
    'ok-tint': '#0a2e1e',
    'ok-emphasis': '#33a370',
    'ok-line': '#14523a',
    'ok-line-strong': '#1b6e4e',
    error: '#ff4d4d',
    'error-strong': '#ff8585',
    'error-contrast': '#ffbdbd',
    'error-muted': '#ff6666',
    'error-solid': '#e53e3e',
    'error-tint': '#330d0d',
    'error-tint-strong': '#4d1414',
    'error-emphasis': '#cc3333',
    'error-line': '#4d1414',
    'error-line-strong': '#6b1d1d',
    warn: '#ffd24d',
    'warn-strong': '#ffe08a',
    'warn-muted': '#e6b32e',
    'warn-solid': '#f5c02e',
    'warn-tint': '#332608',
    'warn-line': '#5c470f',
    'warn-line-strong': '#7a5e14',
    'info-contrast': '#c9b8f0',
    'info-tint': '#1f1433',
    'info-line': '#4d3d80',
    'ansi-black': '#8a7a66',
    'ansi-red': '#ff4d4d',
    'ansi-green': '#66d9a3',
    'ansi-yellow': '#ffd24d',
    'ansi-blue': '#99ccff',
    'ansi-magenta': '#cc99e6',
    'ansi-cyan': '#7dd6e8',
    'ansi-white': '#ffe6cc',
    'ansi-bright-black': '#a3927a',
    'ansi-bright-red': '#ff8585',
    'ansi-bright-green': '#99e8c2',
    'ansi-bright-yellow': '#ffe08a',
    'ansi-bright-blue': '#b8d9ff',
    'ansi-bright-magenta': '#dcc2f0',
    'ansi-bright-cyan': '#a8e4f0',
    'ansi-bright-white': '#fff2e0',
  },
  canvas: {
    chartAxis: '#e68a4d',
    chartGrid: '#291a0d',
    cpuStroke: '#ff9900',
    cpuFill: 'rgba(255,153,0,0.10)',
    memoryStroke: '#99ccff',
    memoryFill: 'rgba(153,204,255,0.10)',
    terminalBackground: '#000000',
    terminalForeground: '#ffcc99',
  },
};

export const BUILT_IN_THEMES: Theme[] = [
  { id: 'dark', name: 'Dark', base: 'dark' },
  { id: 'light', name: 'Light', base: 'light' },
  NORD,
  BLADE_RUNNER,
  CYBERPUNK,
  MATRIX,
  STARTREKTOR,
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

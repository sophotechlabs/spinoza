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

export const SKYWALKER: Theme = {
  id: 'skywalker',
  name: 'Skywalker',
  base: 'dark',
  tokens: {
    surface: '#030710',
    'surface-raised': '#071527',
    'surface-active': '#0a2138',
    handle: '#123d66',
    'handle-active': '#ffe81f',
    grip: '#0c2a47',
    'fg-strong': '#d9f2ff',
    fg: '#66c7ff',
    'fg-soft': '#38b0f5',
    'fg-muted': '#189ee8',
    'fg-subtle': '#1490d6',
    'fg-faint': '#0d5c8f',
    edge: '#0c2a47',
    'edge-strong': '#123d66',
    'edge-emphasis': '#1a548c',
    'edge-active': '#ffe81f',
    'idle-solid': '#4a7ba8',
    ok: '#4de864',
    'ok-contrast': '#baffca',
    'ok-solid': '#35d64f',
    'ok-tint': '#052e12',
    'ok-emphasis': '#28b83f',
    'ok-line': '#0d521c',
    'ok-line-strong': '#117026',
    error: '#ff3b3b',
    'error-strong': '#ff9999',
    'error-contrast': '#ffc9c9',
    'error-muted': '#ff6666',
    'error-solid': '#e01a1a',
    'error-tint': '#3d0808',
    'error-tint-strong': '#571010',
    'error-emphasis': '#c21414',
    'error-line': '#571010',
    'error-line-strong': '#701717',
    warn: '#ffe81f',
    'warn-strong': '#fff27a',
    'warn-muted': '#d9c40e',
    'warn-solid': '#f0d90f',
    'warn-tint': '#332b04',
    'warn-line': '#574c08',
    'warn-line-strong': '#74660c',
    'info-contrast': '#b8ecff',
    'info-tint': '#06283d',
    'info-line': '#1264a0',
    'ansi-black': '#5c85a3',
    'ansi-red': '#ff3b3b',
    'ansi-green': '#4de864',
    'ansi-yellow': '#ffe81f',
    'ansi-blue': '#3d9aff',
    'ansi-magenta': '#c77dff',
    'ansi-cyan': '#5ce0ff',
    'ansi-white': '#d9f2ff',
    'ansi-bright-black': '#7aa3bd',
    'ansi-bright-red': '#ff7a7a',
    'ansi-bright-green': '#85f59a',
    'ansi-bright-yellow': '#fff27a',
    'ansi-bright-blue': '#7abaff',
    'ansi-bright-magenta': '#dba8ff',
    'ansi-bright-cyan': '#a3ecff',
    'ansi-bright-white': '#f0faff',
  },
  canvas: {
    chartAxis: '#1490d6',
    chartGrid: '#081c30',
    cpuStroke: '#66c7ff',
    cpuFill: 'rgba(102,199,255,0.10)',
    memoryStroke: '#ffe81f',
    memoryFill: 'rgba(255,232,31,0.08)',
    terminalBackground: '#010409',
    terminalForeground: '#66c7ff',
  },
};

export const STARTREKTOR: Theme = {
  id: 'startrektor',
  name: 'Startrektor',
  base: 'dark',
  tokens: {
    surface: '#060a12',
    'surface-raised': '#0a1220',
    'surface-active': '#101d33',
    handle: '#1c3252',
    'handle-active': '#ff8a5c',
    grip: '#14243d',
    'fg-strong': '#e8f6ff',
    fg: '#b8dcec',
    'fg-soft': '#93c4dc',
    'fg-muted': '#74b0c8',
    'fg-subtle': '#5da0bd',
    'fg-faint': '#33607a',
    edge: '#14243d',
    'edge-strong': '#1c3252',
    'edge-emphasis': '#264570',
    'edge-active': '#ff8a5c',
    'idle-solid': '#5d7d99',
    ok: '#3fe0a8',
    'ok-contrast': '#b3f5dc',
    'ok-solid': '#2bc990',
    'ok-tint': '#063024',
    'ok-emphasis': '#21ab7a',
    'ok-line': '#0f4d3a',
    'ok-line-strong': '#14684e',
    error: '#ff4455',
    'error-strong': '#ff97a1',
    'error-contrast': '#ffc9cf',
    'error-muted': '#ff6b78',
    'error-solid': '#e02a3c',
    'error-tint': '#380a10',
    'error-tint-strong': '#52101a',
    'error-emphasis': '#c21f30',
    'error-line': '#52101a',
    'error-line-strong': '#701824',
    warn: '#ffb52e',
    'warn-strong': '#ffd075',
    'warn-muted': '#e69c14',
    'warn-solid': '#f5a81f',
    'warn-tint': '#33230a',
    'warn-line': '#573d0e',
    'warn-line-strong': '#755413',
    'info-contrast': '#9be0f5',
    'info-tint': '#0a2c3d',
    'info-line': '#1a6480',
    'ansi-black': '#6e8ba3',
    'ansi-red': '#ff4455',
    'ansi-green': '#3fe0a8',
    'ansi-yellow': '#ffb52e',
    'ansi-blue': '#5c9eff',
    'ansi-magenta': '#c98aff',
    'ansi-cyan': '#54d6f0',
    'ansi-white': '#d8ecf7',
    'ansi-bright-black': '#8aa7bd',
    'ansi-bright-red': '#ff7a85',
    'ansi-bright-green': '#7df0c4',
    'ansi-bright-yellow': '#ffcf70',
    'ansi-bright-blue': '#8cbcff',
    'ansi-bright-magenta': '#ddb0ff',
    'ansi-bright-cyan': '#92e6fa',
    'ansi-bright-white': '#f0f9ff',
  },
  canvas: {
    chartAxis: '#5da0bd',
    chartGrid: '#101c2e',
    cpuStroke: '#ff8a5c',
    cpuFill: 'rgba(255,138,92,0.10)',
    memoryStroke: '#54d6f0',
    memoryFill: 'rgba(84,214,240,0.10)',
    terminalBackground: '#040810',
    terminalForeground: '#b8dcec',
  },
};

export const BORG: Theme = {
  id: 'borg',
  name: 'Borg',
  base: 'dark',
  tokens: {
    surface: '#04100c',
    'surface-raised': '#081a14',
    'surface-active': '#0d2b21',
    handle: '#154a38',
    'handle-active': '#ff8c5a',
    grip: '#0f3528',
    'fg-strong': '#d4fff0',
    fg: '#8ff5d2',
    'fg-soft': '#62e6bb',
    'fg-muted': '#3bd1a4',
    'fg-subtle': '#2bb88f',
    'fg-faint': '#14705a',
    edge: '#0f3528',
    'edge-strong': '#154a38',
    'edge-emphasis': '#1d654c',
    'edge-active': '#ff8c5a',
    'idle-solid': '#4a8a76',
    ok: '#2bff9e',
    'ok-contrast': '#b8ffdf',
    'ok-solid': '#14e688',
    'ok-tint': '#04301e',
    'ok-emphasis': '#10c273',
    'ok-line': '#0a5138',
    'ok-line-strong': '#0d6e4c',
    error: '#ff5c47',
    'error-strong': '#ffa38c',
    'error-contrast': '#ffcbba',
    'error-muted': '#ff7d64',
    'error-solid': '#e03a24',
    'error-tint': '#331008',
    'error-tint-strong': '#4d1a0e',
    'error-emphasis': '#c22e1a',
    'error-line': '#4d1a0e',
    'error-line-strong': '#662413',
    warn: '#ffc233',
    'warn-strong': '#ffd980',
    'warn-muted': '#e0a81f',
    'warn-solid': '#f0b524',
    'warn-tint': '#332608',
    'warn-line': '#57420e',
    'warn-line-strong': '#755a13',
    'info-contrast': '#ffcdb0',
    'info-tint': '#33190c',
    'info-line': '#7a4526',
    'ansi-black': '#4a8a76',
    'ansi-red': '#ff5c47',
    'ansi-green': '#2bff9e',
    'ansi-yellow': '#ffc233',
    'ansi-blue': '#5cb8e6',
    'ansi-magenta': '#e8a0c8',
    'ansi-cyan': '#54e6d6',
    'ansi-white': '#b8f5e2',
    'ansi-bright-black': '#66a893',
    'ansi-bright-red': '#ff8c76',
    'ansi-bright-green': '#7dffbe',
    'ansi-bright-yellow': '#ffd980',
    'ansi-bright-blue': '#8ccff0',
    'ansi-bright-magenta': '#f0bcd8',
    'ansi-bright-cyan': '#85f0e2',
    'ansi-bright-white': '#d4fff0',
  },
  canvas: {
    chartAxis: '#2bb88f',
    chartGrid: '#0c2419',
    cpuStroke: '#ff8c5a',
    cpuFill: 'rgba(255,140,90,0.10)',
    memoryStroke: '#2bff9e',
    memoryFill: 'rgba(43,255,158,0.08)',
    terminalBackground: '#030c09',
    terminalForeground: '#8ff5d2',
  },
};

export const BUILT_IN_THEMES: Theme[] = [
  { id: 'dark', name: 'Dark', base: 'dark' },
  { id: 'light', name: 'Light', base: 'light' },
  NORD,
  BLADE_RUNNER,
  BORG,
  CYBERPUNK,
  MATRIX,
  SKYWALKER,
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

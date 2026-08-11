import type { CanvasColors, Theme, ThemeBase } from './theme';
import { toHex } from './contrast';

const CANVAS_COLORS: Record<ThemeBase, CanvasColors> = {
  dark: {
    chartAxis: '#737373',
    chartGrid: '#262626',
    cpuStroke: '#4ade80',
    cpuFill: 'rgba(74,222,128,0.12)',
    memoryStroke: '#60a5fa',
    memoryFill: 'rgba(96,165,250,0.12)',
    terminalBackground: '#0a0a0a',
    terminalForeground: '#d4d4d4',
  },
  light: {
    chartAxis: '#525252',
    chartGrid: '#e5e5e5',
    cpuStroke: '#16a34a',
    cpuFill: 'rgba(22,163,74,0.12)',
    memoryStroke: '#2563eb',
    memoryFill: 'rgba(37,99,235,0.12)',
    terminalBackground: '#ffffff',
    terminalForeground: '#404040',
  },
};

export function canvasColors(theme: Theme): CanvasColors {
  return { ...CANVAS_COLORS[theme.base], ...theme.canvas };
}

export interface EditorTheme {
  name: string;
  base: ThemeBase;
  background: string;
  foreground: string;
}

function tokenHex(theme: Theme, token: string): string | null {
  const value = theme.tokens?.[token];
  if (value === undefined) {
    return null;
  }
  return toHex(value);
}

export function editorTheme(theme: Theme): EditorTheme {
  const colors = canvasColors(theme);
  let background = colors.terminalBackground;
  const surface = tokenHex(theme, 'surface');
  if (surface !== null) {
    background = surface;
  }
  let foreground = colors.terminalForeground;
  const fg = tokenHex(theme, 'fg');
  if (fg !== null) {
    foreground = fg;
  }
  return { name: `spinoza-${theme.id}`, base: theme.base, background, foreground };
}

export const ANSI_SLOTS = [
  'black',
  'red',
  'green',
  'yellow',
  'blue',
  'magenta',
  'cyan',
  'white',
  'brightBlack',
  'brightRed',
  'brightGreen',
  'brightYellow',
  'brightBlue',
  'brightMagenta',
  'brightCyan',
  'brightWhite',
] as const;

export type AnsiSlot = (typeof ANSI_SLOTS)[number];

export type AnsiPalette = Partial<Record<AnsiSlot, string>>;

export interface XtermTheme extends AnsiPalette {
  background: string;
  foreground: string;
}

function cssName(slot: AnsiSlot): string {
  const dashed = slot.replace(/[A-Z]/g, (letter) => `-${letter.toLowerCase()}`);
  return `--ansi-${dashed}`;
}

export function ansiPalette(read: (name: string) => string): AnsiPalette {
  const palette: AnsiPalette = {};
  for (const slot of ANSI_SLOTS) {
    const hex = toHex(read(cssName(slot)));
    if (hex === null) {
      continue;
    }
    palette[slot] = hex;
  }
  return palette;
}

function fromDocument(name: string): string {
  return window.getComputedStyle(document.documentElement).getPropertyValue(name);
}

export function terminalTheme(theme: Theme, read = fromDocument): XtermTheme {
  const colors = canvasColors(theme);
  return {
    background: colors.terminalBackground,
    foreground: colors.terminalForeground,
    ...ansiPalette(read),
  };
}

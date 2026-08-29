import type { CanvasColors, Theme, ThemeBase } from './theme';
import { contrastRatio, toHex, toRgb } from './contrast';

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

interface EditorRule {
  token: string;
  foreground: string;
}

export interface EditorTheme {
  name: string;
  base: ThemeBase;
  background: string;
  foreground: string;
  colors: Record<string, string>;
  rules: EditorRule[];
}

const AA = 4.5;

const EDITOR_FILLS: [string, string][] = [
  ['editorGutter.background', 'surface'],
  ['editor.lineHighlightBackground', 'surface-raised'],
  ['editor.selectionBackground', 'surface-active'],
  ['editorIndentGuide.background1', 'edge'],
  ['editorIndentGuide.activeBackground1', 'edge-strong'],
  ['editorWhitespace.foreground', 'edge-strong'],
  ['editorWidget.background', 'surface-raised'],
  ['editorWidget.border', 'edge-strong'],
  ['editorSuggestWidget.background', 'surface-raised'],
  ['editorSuggestWidget.border', 'edge-strong'],
  ['editorSuggestWidget.selectedBackground', 'surface-active'],
  ['editorHoverWidget.background', 'surface-raised'],
  ['editorHoverWidget.border', 'edge-strong'],
  ['minimap.background', 'surface'],
  ['scrollbarSlider.background', 'handle'],
  ['scrollbarSlider.hoverBackground', 'handle-active'],
  ['scrollbarSlider.activeBackground', 'edge-active'],
];

const EDITOR_TEXT: [string, string, string][] = [
  ['editorLineNumber.foreground', 'fg-muted', 'surface'],
  ['editorLineNumber.activeForeground', 'fg-strong', 'surface'],
  ['editorCursor.foreground', 'fg-strong', 'surface'],
  ['editorWidget.foreground', 'fg', 'surface-raised'],
  ['editorSuggestWidget.foreground', 'fg', 'surface-raised'],
  ['editorSuggestWidget.selectedForeground', 'fg-strong', 'surface-active'],
  ['editorHoverWidget.foreground', 'fg', 'surface-raised'],
  ['editorError.foreground', 'error', 'surface'],
  ['editorWarning.foreground', 'warn', 'surface'],
];

const EDITOR_RULES: [string, string][] = [
  ['', 'fg'],
  ['comment', 'fg-subtle'],
  ['string', 'ansi-green'],
  ['string.yaml', 'ansi-green'],
  ['number', 'ansi-cyan'],
  ['keyword', 'ansi-blue'],
  ['type', 'ansi-blue'],
  ['tag', 'ansi-blue'],
  ['attribute.name', 'ansi-blue'],
  ['delimiter', 'fg-muted'],
];

function tokenHex(theme: Theme, token: string): string | null {
  const value = theme.tokens?.[token];
  if (value === undefined) {
    return null;
  }
  return toHex(value);
}

function hexOf(rgb: [number, number, number]): string {
  return `#${rgb.map((channel) => channel.toString(16).padStart(2, '0')).join('')}`;
}

function readableOn(theme: Theme, token: string, behind: string, fallback: string): string | null {
  const raw = theme.tokens?.[token];
  if (raw === undefined) {
    return null;
  }
  const ink = toRgb(raw);
  if (ink === null) {
    return null;
  }
  const paper = toRgb(tokenHex(theme, behind) ?? fallback);
  if (paper === null) {
    return hexOf(ink);
  }
  if (contrastRatio(ink, paper) < AA) {
    return fallback;
  }
  return hexOf(ink);
}

function editorColors(theme: Theme, foreground: string): Record<string, string> {
  const mapped: Record<string, string> = {};
  for (const [key, token] of EDITOR_FILLS) {
    const value = tokenHex(theme, token);
    if (value === null) {
      continue;
    }
    mapped[key] = value;
  }
  for (const [key, token, behind] of EDITOR_TEXT) {
    const value = readableOn(theme, token, behind, foreground);
    if (value === null) {
      continue;
    }
    mapped[key] = value;
  }
  return mapped;
}

function editorRules(theme: Theme, foreground: string): EditorRule[] {
  const rules: EditorRule[] = [];
  for (const [token, source] of EDITOR_RULES) {
    const value = readableOn(theme, source, 'surface', foreground);
    if (value === null) {
      continue;
    }
    rules.push({ token, foreground: value.replace('#', '') });
  }
  return rules;
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
  return {
    name: `spinoza-${theme.id}`,
    base: theme.base,
    background,
    foreground,
    colors: editorColors(theme, foreground),
    rules: editorRules(theme, foreground),
  };
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

import type { CanvasColors, Theme, ThemeBase } from './theme';

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

export function terminalTheme(theme: Theme): { background: string; foreground: string } {
  const colors = canvasColors(theme);
  return { background: colors.terminalBackground, foreground: colors.terminalForeground };
}

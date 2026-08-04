export const THEMES = ['dark', 'light', 'system'] as const;

export type ThemePreference = (typeof THEMES)[number];

export type ResolvedTheme = 'dark' | 'light';

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

export function systemTheme(): ResolvedTheme {
  if (window.matchMedia(DARK_QUERY).matches) {
    return 'dark';
  }
  return 'light';
}

export function watchSystemTheme(onChange: (theme: ResolvedTheme) => void): void {
  window.matchMedia(DARK_QUERY).addEventListener('change', (event) => {
    if (event.matches) {
      onChange('dark');
      return;
    }
    onChange('light');
  });
}

export function resolveTheme(preference: ThemePreference, system: ResolvedTheme): ResolvedTheme {
  if (preference === 'system') {
    return system;
  }
  return preference;
}

export function applyTheme(resolved: ResolvedTheme): void {
  document.documentElement.dataset.theme = resolved;
  document.documentElement.style.colorScheme = resolved;
}

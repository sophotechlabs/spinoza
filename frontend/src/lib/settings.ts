export const LOG_VIEWS = ['pretty', 'raw'] as const;

export type LogView = (typeof LOG_VIEWS)[number];

export const SETTINGS_KEY = 'spinoza.settings.v1';

interface Settings {
  logView: LogView;
}

const DEFAULTS: Settings = { logView: 'pretty' };

export function parseSettings(raw: string | null): Settings {
  if (raw === null) {
    return { ...DEFAULTS };
  }
  let parsed: unknown = null;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return { ...DEFAULTS };
  }
  if (typeof parsed !== 'object' || parsed === null) {
    return { ...DEFAULTS };
  }
  const stored = parsed as Record<string, unknown>;
  const settings = { ...DEFAULTS };
  for (const view of LOG_VIEWS) {
    if (stored.logView === view) {
      settings.logView = view;
    }
  }
  return settings;
}

export function readSettings(): Settings {
  try {
    return parseSettings(window.localStorage.getItem(SETTINGS_KEY));
  } catch {
    return { ...DEFAULTS };
  }
}

export function writeSettings(settings: Settings): void {
  try {
    window.localStorage.setItem(SETTINGS_KEY, JSON.stringify(settings));
  } catch {
    return;
  }
}

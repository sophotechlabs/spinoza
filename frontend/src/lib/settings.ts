import { readStored, writeStored } from './persist';
export const LOG_VIEWS = ['pretty', 'raw'] as const;

export type LogView = (typeof LOG_VIEWS)[number];

export const SETTINGS_KEY = 'spinoza.settings.v1';

interface Settings {
  logView: LogView;
  screenReader: boolean;
}

const DEFAULTS: Settings = { logView: 'pretty', screenReader: false };

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
  if (typeof stored.screenReader === 'boolean') {
    settings.screenReader = stored.screenReader;
  }
  return settings;
}

export function readSettings(): Settings {
  return parseSettings(readStored(SETTINGS_KEY));
}

export function writeSettings(settings: Settings): void {
  writeStored(SETTINGS_KEY, JSON.stringify(settings));
}

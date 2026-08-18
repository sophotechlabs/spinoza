import { readStored, writeStored } from './persist';
export const LOG_VIEWS = ['pretty', 'raw'] as const;

export type LogView = (typeof LOG_VIEWS)[number];

export const NAMESPACE_STARTS = ['all', 'default'] as const;

export type NamespaceStart = (typeof NAMESPACE_STARTS)[number];

export const SETTINGS_KEY = 'spinoza.settings.v1';

export interface Settings {
  logView: LogView;
  screenReader: boolean;
  namespaceStart: NamespaceStart;
  namespaceAsked: boolean;
}

const DEFAULTS: Settings = {
  logView: 'pretty',
  screenReader: false,
  namespaceStart: 'all',
  namespaceAsked: false,
};

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
  for (const start of NAMESPACE_STARTS) {
    if (stored.namespaceStart === start) {
      settings.namespaceStart = start;
    }
  }
  if (typeof stored.namespaceAsked === 'boolean') {
    settings.namespaceAsked = stored.namespaceAsked;
  }
  return settings;
}

export function readSettings(): Settings {
  return parseSettings(readStored(SETTINGS_KEY));
}

export function writeSettings(settings: Settings): void {
  writeStored(SETTINGS_KEY, JSON.stringify(settings));
}

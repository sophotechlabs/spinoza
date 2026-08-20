import { flush, readStored, writeStored } from './persist';
export const LOG_VIEWS = ['pretty', 'raw'] as const;

export type LogView = (typeof LOG_VIEWS)[number];

export const NAMESPACE_STARTS = ['all', 'default'] as const;

export type NamespaceStart = (typeof NAMESPACE_STARTS)[number];

export const EVERY_NAMESPACE: NamespaceStart = 'all';

export const ONLY_DEFAULT: NamespaceStart = 'default';

export const SETTINGS_KEY = 'spinoza.settings.v1';

export interface Settings {
  logView: LogView;
  screenReader: boolean;
  namespaceStart: NamespaceStart;
  namespaceStarts: Partial<Record<string, NamespaceStart>>;
}

const DEFAULTS: Settings = {
  logView: 'pretty',
  screenReader: false,
  namespaceStart: 'all',
  namespaceStarts: {},
};

function parseStarts(value: unknown): Partial<Record<string, NamespaceStart>> {
  const out: Partial<Record<string, NamespaceStart>> = {};
  if (typeof value !== 'object' || value === null) {
    return out;
  }
  for (const [context, choice] of Object.entries(value as Record<string, unknown>)) {
    for (const start of NAMESPACE_STARTS) {
      if (choice === start) {
        out[context] = start;
      }
    }
  }
  return out;
}

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
  settings.namespaceStarts = parseStarts(stored.namespaceStarts);
  return settings;
}

export function readSettings(): Settings {
  return parseSettings(readStored(SETTINGS_KEY));
}

export function writeSettings(settings: Settings): void {
  writeStored(SETTINGS_KEY, JSON.stringify(settings));
}

export const NODE_SHELL_KEY = 'spinoza.nodeshell.v1';

const ON = 'on';

const OFF = 'off';

export function readNodeShell(): boolean {
  return readStored(NODE_SHELL_KEY) === ON;
}

export function writeNodeShell(enabled: boolean): Promise<void> {
  if (enabled) {
    writeStored(NODE_SHELL_KEY, ON);
  } else {
    writeStored(NODE_SHELL_KEY, OFF);
  }
  return flush();
}

import { flush, readStored, writeStored } from './persist';
export const LOG_VIEWS = ['pretty', 'raw'] as const;

export type LogView = (typeof LOG_VIEWS)[number];

export const NAMESPACE_STARTS = ['all', 'default'] as const;

export type NamespaceStart = (typeof NAMESPACE_STARTS)[number];

export const EVERY_NAMESPACE: NamespaceStart = 'all';

export const ONLY_DEFAULT: NamespaceStart = 'default';

export const CHECK_INTERVALS = [15, 30, 60, 300] as const;

export type CheckInterval = (typeof CHECK_INTERVALS)[number];

export const SETTINGS_KEY = 'spinoza.settings.v1';

export const SEVERITY_FLOORS = ['', 'low', 'medium', 'high'] as const;

export type SeverityFloor = (typeof SEVERITY_FLOORS)[number];

export interface Settings {
  logView: LogView;
  screenReader: boolean;
  namespaceStart: NamespaceStart;
  namespaceStarts: Partial<Record<string, NamespaceStart>>;
  checksInterval: CheckInterval;
  checksDisabled: string[];
  checksSkipNamespaces: string[];
  checksMinSeverity: SeverityFloor;
  checksWholeCluster: boolean;
  checksEveryKind: boolean;
}

const DEFAULTS: Settings = {
  logView: 'pretty',
  screenReader: false,
  namespaceStart: 'all',
  namespaceStarts: {},
  checksInterval: 60,
  checksDisabled: [],
  checksSkipNamespaces: [],
  checksMinSeverity: '',
  checksWholeCluster: true,
  checksEveryKind: false,
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
  for (const interval of CHECK_INTERVALS) {
    if (stored.checksInterval === interval) {
      settings.checksInterval = interval;
    }
  }
  for (const floor of SEVERITY_FLOORS) {
    if (stored.checksMinSeverity === floor) {
      settings.checksMinSeverity = floor;
    }
  }
  if (typeof stored.checksWholeCluster === 'boolean') {
    settings.checksWholeCluster = stored.checksWholeCluster;
  }
  if (typeof stored.checksEveryKind === 'boolean') {
    settings.checksEveryKind = stored.checksEveryKind;
  }
  settings.checksDisabled = parseNames(stored.checksDisabled);
  settings.checksSkipNamespaces = parseNames(stored.checksSkipNamespaces);
  settings.namespaceStarts = parseStarts(stored.namespaceStarts);
  return settings;
}

function parseNames(raw: unknown): string[] {
  if (!Array.isArray(raw)) {
    return [];
  }
  return raw.filter((entry): entry is string => typeof entry === 'string' && entry !== '');
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

export const UPDATE_CHECK_KEY = 'spinoza.update.check.v1';

// On until it is turned off, so a fresh install has never been asked.
export function readUpdateCheck(): boolean {
  return readStored(UPDATE_CHECK_KEY) !== OFF;
}

export function writeUpdateCheck(enabled: boolean): Promise<void> {
  if (enabled) {
    writeStored(UPDATE_CHECK_KEY, ON);
  } else {
    writeStored(UPDATE_CHECK_KEY, OFF);
  }
  return flush();
}

export const CHECK_RULES_KEY = 'spinoza.checks.rules.v1';

export function readCheckRules(): string {
  return readStored(CHECK_RULES_KEY) ?? '';
}

export function writeCheckRules(rules: string): Promise<void> {
  writeStored(CHECK_RULES_KEY, rules);
  return flush();
}

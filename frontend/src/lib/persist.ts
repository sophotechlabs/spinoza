import type { Settings } from './types';
import { request } from './http';

export const SETTINGS_PATH = '/api/settings';

export const SAVE_DELAY_MS = 400;

const KEYS = [
  'spinoza.theme.v1',
  'spinoza.themes.v1',
  'spinoza.panels.v1',
  'spinoza.layout.v1',
  'spinoza.tables.v1',
  'spinoza.sidebar.v1',
  'spinoza.settings.v1',
  'spinoza.painted.v1',
  'spinoza.nodeshell.v1',
  'spinoza.columns.v1',
  'spinoza.update.check.v1',
  'spinoza.checks.rules.v1',
];

declare global {
  interface Window {
    __SPINOZA_SETTINGS__?: string;
    __spinozaSettings__?: Map<string, string>;
  }
}

function cache(): Map<string, string> {
  const held = window.__spinozaSettings__;
  if (held !== undefined) {
    return held;
  }
  const fresh = new Map<string, string>();
  window.__spinozaSettings__ = fresh;
  return fresh;
}

function replace(next: Map<string, string>): void {
  window.__spinozaSettings__ = next;
}

let timer: ReturnType<typeof setTimeout> | null = null;
let saving = false;

const changed = new Set<string>();

export function startSaving(): void {
  saving = true;
}

export function stopSaving(): void {
  saving = false;
}

export function isSaving(): boolean {
  return saving;
}

function fromServer(): Map<string, string> | null {
  const raw = window.__SPINOZA_SETTINGS__;
  if (raw === undefined) {
    return null;
  }
  let parsed: unknown = null;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return null;
  }
  if (typeof parsed !== 'object' || parsed === null) {
    return null;
  }
  const found = new Map<string, string>();
  for (const [key, value] of Object.entries(parsed)) {
    if (typeof value === 'string') {
      found.set(key, value);
    }
  }
  return found;
}

function fromBrowser(): Map<string, string> {
  const found = new Map<string, string>();
  for (const key of KEYS) {
    let stored: string | null = null;
    try {
      stored = window.localStorage.getItem(key);
    } catch {
      stored = null;
    }
    if (stored !== null) {
      found.set(key, stored);
    }
  }
  return found;
}

export function hydrate(): void {
  const served = fromServer();
  if (served === null) {
    replace(fromBrowser());
    return;
  }
  if (served.size > 0) {
    replace(served);
    return;
  }
  const local = fromBrowser();
  replace(local);
  if (local.size === 0) {
    return;
  }
  for (const key of local.keys()) {
    changed.add(key);
  }
  void save();
}

export function readStored(key: string): string | null {
  return cache().get(key) ?? null;
}

export function writeStored(key: string, value: string): void {
  cache().set(key, value);
  changed.add(key);
  schedule();
}

export function storedKeys(): string[] {
  return [...cache().keys()];
}

function schedule(): void {
  if (!saving) {
    return;
  }
  if (timer !== null) {
    clearTimeout(timer);
  }
  timer = setTimeout(() => {
    timer = null;
    void save();
  }, SAVE_DELAY_MS);
}

export async function refresh(): Promise<boolean> {
  let body: unknown = null;
  try {
    const response = await request(SETTINGS_PATH);
    if (!response.ok) {
      return false;
    }
    body = await response.json();
  } catch {
    return false;
  }
  const found = valuesOf(body);
  if (found === null) {
    return false;
  }
  for (const key of changed) {
    const held = cache().get(key);
    if (held !== undefined) {
      found.set(key, held);
    }
  }
  if (same(cache(), found)) {
    return false;
  }
  replace(found);
  return true;
}

function valuesOf(body: unknown): Map<string, string> | null {
  if (typeof body !== 'object' || body === null) {
    return null;
  }
  const values = (body as { values?: unknown }).values;
  if (typeof values !== 'object' || values === null) {
    return null;
  }
  const found = new Map<string, string>();
  for (const [key, value] of Object.entries(values)) {
    if (typeof value === 'string') {
      found.set(key, value);
    }
  }
  return found;
}

function same(held: Map<string, string>, found: Map<string, string>): boolean {
  if (held.size !== found.size) {
    return false;
  }
  for (const [key, value] of held) {
    if (found.get(key) !== value) {
      return false;
    }
  }
  return true;
}

export function save(): Promise<void> {
  if (!saving) {
    return Promise.resolve();
  }
  const sending = pending();
  if (Object.keys(sending).length === 0) {
    return Promise.resolve();
  }
  const settings: Settings = { values: sending };
  return request(SETTINGS_PATH, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(settings),
  })
    .then((response) => {
      if (response.ok) {
        accepted(sending);
      }
    })
    .catch(() => undefined);
}

function pending(): Record<string, string> {
  const out: Record<string, string> = {};
  for (const key of changed) {
    const value = cache().get(key);
    if (value !== undefined) {
      out[key] = value;
    }
  }
  return out;
}

function accepted(sent: Record<string, string>): void {
  for (const [key, value] of Object.entries(sent)) {
    if (cache().get(key) === value) {
      changed.delete(key);
    }
  }
}

export function flush(): Promise<void> {
  if (timer !== null) {
    clearTimeout(timer);
    timer = null;
  }
  return save();
}

export function resetStored(): void {
  saving = false;
  changed.clear();
  replace(new Map<string, string>());
  if (timer !== null) {
    clearTimeout(timer);
    timer = null;
  }
}

if (window.__spinozaSettings__ === undefined) {
  hydrate();
}

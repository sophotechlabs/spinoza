import type { Theme, ThemeBase } from './theme';
import { BUILT_IN_THEMES, CANVAS_NAMES, TOKEN_NAMES } from './theme';
import { contrastWarnings } from './contrast';
import { readStored, writeStored } from './persist';

export const CUSTOM_THEMES_KEY = 'spinoza.themes.v1';

const HEX = /^#([0-9a-f]{3,4}|[0-9a-f]{6}|[0-9a-f]{8})$/i;
const FUNCTIONAL = /^(rgb|rgba|hsl|hsla|hwb|lab|lch|oklab|oklch|color)\([^()]+\)$/i;

export function isColor(value: string): boolean {
  const text = value.trim();
  if (HEX.test(text)) {
    return true;
  }
  return FUNCTIONAL.test(text);
}

function asRecord(value: unknown): Record<string, unknown> | null {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    return null;
  }
  return value as Record<string, unknown>;
}

function checkMap(
  raw: unknown,
  allowed: string[],
  label: string,
  errors: string[],
): Record<string, string> | undefined {
  if (raw === undefined) {
    return undefined;
  }
  const fields = asRecord(raw);
  if (fields === null) {
    errors.push(`${label} must be an object`);
    return undefined;
  }
  const out: Record<string, string> = {};
  for (const [key, value] of Object.entries(fields)) {
    if (!allowed.includes(key)) {
      errors.push(`${label}: "${key}" is not a known name`);
      continue;
    }
    if (typeof value !== 'string' || !isColor(value)) {
      errors.push(`${label}: "${key}" is not a colour this understands`);
      continue;
    }
    out[key] = value.trim();
  }
  return out;
}

export interface ThemeCheck {
  theme: Theme | null;
  errors: string[];
  warnings: string[];
}

const BUILT_IN_IDS = BUILT_IN_THEMES.map((theme) => theme.id);

export function validateTheme(raw: unknown): ThemeCheck {
  const errors: string[] = [];
  const warnings: string[] = [];
  const fields = asRecord(raw);
  if (fields === null) {
    return { theme: null, errors: ['a theme must be a JSON object'], warnings };
  }

  const id = fields.id;
  if (typeof id !== 'string' || id.trim() === '') {
    errors.push('id is required');
  }
  if (typeof id === 'string' && BUILT_IN_IDS.includes(id)) {
    errors.push(`id "${id}" is reserved for a built-in theme`);
  }

  const name = fields.name;
  if (typeof name !== 'string' || name.trim() === '') {
    errors.push('name is required');
  }

  const base = fields.base;
  if (base !== 'dark' && base !== 'light') {
    errors.push('base must be "dark" or "light"');
  }

  const tokens = checkMap(fields.tokens, TOKEN_NAMES, 'tokens', errors);
  const canvas = checkMap(fields.canvas, CANVAS_NAMES, 'canvas', errors);

  if (tokens !== undefined && Object.keys(tokens).length > 0 && !Object.hasOwn(tokens, 'surface')) {
    warnings.push('this theme does not set surface, so it inherits the background of its base');
  }
  if (tokens !== undefined) {
    warnings.push(...contrastWarnings(tokens));
  }

  if (errors.length > 0) {
    return { theme: null, errors, warnings };
  }

  const theme: Theme = {
    id: (id as string).trim(),
    name: (name as string).trim(),
    base: base as ThemeBase,
  };
  if (tokens !== undefined) {
    theme.tokens = tokens;
  }
  if (canvas !== undefined) {
    theme.canvas = canvas;
  }
  return { theme, errors, warnings };
}

export function parseCustomThemes(raw: string | null): Theme[] {
  if (raw === null) {
    return [];
  }
  let parsed: unknown = null;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return [];
  }
  if (!Array.isArray(parsed)) {
    return [];
  }
  const themes: Theme[] = [];
  for (const entry of parsed) {
    const checked = validateTheme(entry);
    if (checked.theme !== null) {
      themes.push(checked.theme);
    }
  }
  return themes;
}

export function readCustomThemes(): Theme[] {
  return parseCustomThemes(readStored(CUSTOM_THEMES_KEY));
}

export function writeCustomThemes(themes: Theme[]): void {
  writeStored(CUSTOM_THEMES_KEY, JSON.stringify(themes));
}

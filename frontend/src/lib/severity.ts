import type { Severity } from './types';

export const SEVERITY_ORDER: Severity[] = ['high', 'medium', 'low'];

const CLASSES = new Map<string, string>([
  ['high', 'text-error'],
  ['medium', 'text-warn'],
  ['low', 'text-fg-muted'],
]);

const LABELS = new Map<string, string>([
  ['high', 'high'],
  ['medium', 'medium'],
  ['low', 'low'],
]);

export function severityClass(severity: string): string {
  return CLASSES.get(severity) ?? 'text-fg-muted';
}

export function severityLabel(severity: string): string {
  return LABELS.get(severity) ?? severity;
}

export function severityRank(severity: string): number {
  const at = SEVERITY_ORDER.findIndex((one) => one === severity);
  if (at < 0) {
    return SEVERITY_ORDER.length;
  }
  return at;
}

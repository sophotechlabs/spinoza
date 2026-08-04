export type LogSeverity = 'error' | 'warn' | 'debug' | 'info';

export interface LogSegment {
  text: string;
  className: string;
}

const SEVERITY_CLASS: Record<LogSeverity, string> = {
  error: 'text-error',
  warn: 'text-warn',
  debug: 'text-fg-subtle',
  info: '',
};

const TIMESTAMP = /^(?:time=)?(\d{4}[-/]\d{2}[-/]\d{2}[T ]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?Z?)\s*/;

const LEVEL_HINT = 60;

const PATTERNS: { pattern: RegExp; severity: LogSeverity }[] = [
  { pattern: /"level"\s*:\s*"(?:error|fatal|panic|critical)"/i, severity: 'error' },
  { pattern: /"level"\s*:\s*"(?:warn|warning)"/i, severity: 'warn' },
  { pattern: /"level"\s*:\s*"(?:debug|trace)"/i, severity: 'debug' },
  { pattern: /\blevel=(?:error|fatal|panic|critical)\b/i, severity: 'error' },
  { pattern: /\blevel=(?:warn|warning)\b/i, severity: 'warn' },
  { pattern: /\blevel=(?:debug|trace)\b/i, severity: 'debug' },
  { pattern: /\[(?:E|F)\]/, severity: 'error' },
  { pattern: /\[W\]/, severity: 'warn' },
  { pattern: /\[(?:D|T)\]/, severity: 'debug' },
  { pattern: /\b(?:ERROR|ERR|FATAL|PANIC|SEVERE)\b/, severity: 'error' },
  { pattern: /\b(?:WARN|WARNING)\b/, severity: 'warn' },
  { pattern: /\b(?:DEBUG|TRACE|FINE)\b/, severity: 'debug' },
];

export function severityOf(line: string): LogSeverity {
  const head = line.slice(0, LEVEL_HINT);
  for (const entry of PATTERNS) {
    if (entry.pattern.test(head)) {
      return entry.severity;
    }
  }
  return 'info';
}

export function severityClass(severity: LogSeverity): string {
  return SEVERITY_CLASS[severity];
}

const ESC = String.fromCharCode(27) + '[';

const SGR = new RegExp(`${String.fromCharCode(27)}\\[([0-9;]*)m`, 'g');

const ANSI_CLASS: Record<string, string | undefined> = {
  '30': 'text-ansi-black',
  '31': 'text-ansi-red',
  '32': 'text-ansi-green',
  '33': 'text-ansi-yellow',
  '34': 'text-ansi-blue',
  '35': 'text-ansi-magenta',
  '36': 'text-ansi-cyan',
  '37': 'text-ansi-white',
  '90': 'text-ansi-bright-black',
  '91': 'text-ansi-bright-red',
  '92': 'text-ansi-bright-green',
  '93': 'text-ansi-bright-yellow',
  '94': 'text-ansi-bright-blue',
  '95': 'text-ansi-bright-magenta',
  '96': 'text-ansi-bright-cyan',
  '97': 'text-ansi-bright-white',
};

export function hasAnsi(line: string): boolean {
  return line.includes(ESC);
}

function classForCodes(codes: string[], current: string): string {
  let className = current;
  for (const code of codes) {
    if (code === '' || code === '0') {
      className = '';
      continue;
    }
    const mapped = ANSI_CLASS[code];
    if (mapped !== undefined) {
      className = mapped;
    }
  }
  return className;
}

function ansiSegments(line: string): LogSegment[] {
  const segments: LogSegment[] = [];
  let className = '';
  let at = 0;
  SGR.lastIndex = 0;
  let match = SGR.exec(line);
  while (match !== null) {
    const text = line.slice(at, match.index);
    if (text !== '') {
      segments.push({ text, className });
    }
    className = classForCodes(match[1].split(';'), className);
    at = match.index + match[0].length;
    match = SGR.exec(line);
  }
  const tail = line.slice(at);
  if (tail !== '') {
    segments.push({ text: tail, className });
  }
  return segments;
}

export function segmentsOf(line: string, withTime = true): LogSegment[] {
  if (hasAnsi(line)) {
    return ansiSegments(line);
  }
  const severity = severityOf(line);
  const body = severityClass(severity);
  const stamp = TIMESTAMP.exec(line);
  if (stamp === null) {
    return [{ text: line, className: body }];
  }
  const rest = { text: line.slice(stamp[0].length), className: body };
  if (!withTime) {
    return [rest];
  }
  return [{ text: stamp[0], className: 'text-fg-subtle' }, rest];
}

const LEVEL_KEYS = ['level', 'severity', 'lvl'];
const TIME_KEYS = ['ts', 'time', 'timestamp', '@timestamp'];
const MESSAGE_KEYS = ['msg', 'message'];

const LEVEL_SEVERITY: Record<string, LogSeverity | undefined> = {
  error: 'error',
  err: 'error',
  fatal: 'error',
  panic: 'error',
  critical: 'error',
  warn: 'warn',
  warning: 'warn',
  debug: 'debug',
  trace: 'debug',
  info: 'info',
  information: 'info',
  notice: 'info',
};

const INDENT = '\n          ';

function parseObject(line: string): Record<string, unknown> | null {
  const text = line.trim();
  if (!text.startsWith('{') || !text.endsWith('}')) {
    return null;
  }
  let parsed: unknown = null;
  try {
    parsed = JSON.parse(text);
  } catch {
    return null;
  }
  return parsed as Record<string, unknown>;
}

function take(fields: Record<string, unknown>, keys: string[], used: Set<string>): string | null {
  for (const key of keys) {
    const value = fields[key];
    if (typeof value === 'string' && value !== '') {
      used.add(key);
      return value;
    }
    if (typeof value === 'number') {
      used.add(key);
      return String(value);
    }
  }
  return null;
}

export function clockOf(raw: string | null): string {
  if (raw === null) {
    return '';
  }
  const iso = /\d{2}:\d{2}:\d{2}/.exec(raw);
  if (iso !== null) {
    return iso[0];
  }
  const epoch = Number(raw);
  if (Number.isNaN(epoch)) {
    return '';
  }
  let millis = epoch * 1000;
  if (epoch > 1e11) {
    millis = epoch;
  }
  return new Date(millis).toISOString().slice(11, 19);
}

function severityFrom(level: string | null): LogSeverity {
  if (level === null) {
    return 'info';
  }
  const known = LEVEL_SEVERITY[level.toLowerCase()];
  if (known === undefined) {
    return 'info';
  }
  return known;
}

function rest(fields: Record<string, unknown>, used: Set<string>): string {
  const parts: string[] = [];
  for (const [key, value] of Object.entries(fields)) {
    if (used.has(key)) {
      continue;
    }
    if (typeof value === 'string') {
      parts.push(`${key}=${value}`);
      continue;
    }
    parts.push(`${key}=${JSON.stringify(value)}`);
  }
  return parts.join(' ');
}

export function rawSegments(line: string): LogSegment[] {
  return [{ text: line, className: '' }];
}

export const SEGMENT_CACHE_LIMIT = 12000;

const stamped = new Map<string, LogSegment[]>();
const bare = new Map<string, LogSegment[]>();

function cacheFor(withTime: boolean): Map<string, LogSegment[]> {
  if (withTime) {
    return stamped;
  }
  return bare;
}

export function cachedSegments(line: string, withTime = true): LogSegment[] {
  const cache = cacheFor(withTime);
  const hit = cache.get(line);
  if (hit !== undefined) {
    return hit;
  }
  const segments = prettySegments(line, withTime);
  if (cache.size >= SEGMENT_CACHE_LIMIT) {
    cache.clear();
  }
  cache.set(line, segments);
  return segments;
}

export function forgetSegments(): void {
  stamped.clear();
  bare.clear();
}

export function prettySegments(line: string, withTime = true): LogSegment[] {
  const fields = parseObject(line);
  if (fields === null) {
    return segmentsOf(line, withTime);
  }
  const used = new Set<string>();
  const level = take(fields, LEVEL_KEYS, used);
  const clock = clockOf(take(fields, TIME_KEYS, used));
  const found = take(fields, MESSAGE_KEYS, used);
  let message = '';
  if (found !== null) {
    message = found;
  }
  const severity = severityFrom(level);
  const segments: LogSegment[] = [];

  if (clock !== '' && withTime) {
    segments.push({ text: `${clock}  `, className: 'text-fg-subtle' });
  }
  if (level !== null) {
    segments.push({ text: level.toUpperCase().padEnd(5), className: severityClass(severity) });
    segments.push({ text: '  ', className: '' });
  }
  segments.push({ text: message, className: severityClass(severity) });

  const tail = rest(fields, used);
  if (tail !== '') {
    segments.push({ text: `${INDENT}${tail}`, className: 'text-fg-subtle' });
  }
  return segments;
}

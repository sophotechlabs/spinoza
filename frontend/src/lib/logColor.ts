export type LogSeverity = 'error' | 'warn' | 'debug' | 'info';

export interface LogSegment {
  text: string;
  className: string;
}

const SEVERITY_CLASS: Record<LogSeverity, string> = {
  error: 'text-red-400',
  warn: 'text-amber-400',
  debug: 'text-neutral-500',
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
  '30': 'text-neutral-600',
  '31': 'text-red-400',
  '32': 'text-green-400',
  '33': 'text-amber-400',
  '34': 'text-sky-400',
  '35': 'text-fuchsia-400',
  '36': 'text-cyan-400',
  '37': 'text-neutral-200',
  '90': 'text-neutral-500',
  '91': 'text-red-300',
  '92': 'text-green-300',
  '93': 'text-amber-300',
  '94': 'text-sky-300',
  '95': 'text-fuchsia-300',
  '96': 'text-cyan-300',
  '97': 'text-neutral-100',
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

export function segmentsOf(line: string): LogSegment[] {
  if (hasAnsi(line)) {
    return ansiSegments(line);
  }
  const severity = severityOf(line);
  const body = severityClass(severity);
  const stamp = TIMESTAMP.exec(line);
  if (stamp === null) {
    return [{ text: line, className: body }];
  }
  return [
    { text: stamp[0], className: 'text-neutral-500' },
    { text: line.slice(stamp[0].length), className: body },
  ];
}

import { since as elapsed } from './time';

const UNKNOWN = 'no answer';

const PHRASES = new Map<string, string>([
  ['timeout', 'timed out'],
  ['refused', 'connection refused'],
  ['tls', 'certificate not trusted'],
  ['dns', 'the address did not resolve'],
  ['unauthorized', 'not authorised'],
  ['other', UNKNOWN],
]);

export function causePhrase(cause: string): string {
  if (cause === '') {
    return '';
  }
  const known = PHRASES.get(cause);
  if (known === undefined) {
    return UNKNOWN;
  }
  return known;
}

export function quietFor(from: number, now: number): string {
  if (from === 0) {
    return '';
  }
  return elapsed(Math.floor((now - from) / 1000));
}

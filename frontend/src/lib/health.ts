import { since as elapsed } from './time';
import { offline } from './feed';
import { useFeedStore } from '../store/feed';
import { useClusterReachable } from '../store/clusterHealth';
import { useSessionExpired } from '../store/session';

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

export function useFeedDown(): boolean {
  const status = useFeedStore((state) => state.status);
  const attempt = useFeedStore((state) => state.attempt);
  return offline(status, attempt);
}

export function useShellExplains(): boolean {
  const down = useFeedDown();
  const reachable = useClusterReachable();
  const expired = useSessionExpired();
  if (expired) {
    return true;
  }
  if (down) {
    return true;
  }
  return !reachable;
}

import { describe, expect, it } from 'vitest';
import { causePhrase, quietFor } from '../../src/lib/health';

describe('putting a plain word to why a cluster is quiet', () => {
  it('names each cause the server can send', () => {
    expect(causePhrase('timeout')).toBe('timed out');
    expect(causePhrase('refused')).toBe('connection refused');
    expect(causePhrase('tls')).toBe('certificate not trusted');
    expect(causePhrase('dns')).toBe('the address did not resolve');
    expect(causePhrase('unauthorized')).toBe('not authorised');
    expect(causePhrase('other')).toBe('no answer');
  });

  it('says nothing when the server named no cause', () => {
    expect(causePhrase('')).toBe('');
  });

  it('falls back rather than showing a word a newer server invented', () => {
    expect(causePhrase('quantum-tunnelling')).toBe('no answer');
  });
});

describe('how long a cluster has been quiet', () => {
  it('counts from the moment it stopped answering', () => {
    const now = 1_700_000_000_000;

    expect(quietFor(now - 42_000, now)).toBe('42s');
    expect(quietFor(now - 300_000, now)).toBe('5m');
  });

  it('says nothing when nobody recorded the moment', () => {
    expect(quietFor(0, 1_700_000_000_000)).toBe('');
  });
});

import { describe, expect, it } from 'vitest';
import { renderHook } from '@testing-library/react';
import { causePhrase, quietFor, useShellExplains } from '../../src/lib/health';
import { useFeedStore } from '../../src/store/feed';
import { reportHealth } from '../../src/store/clusterHealth';
import { expireSession } from '../../src/store/session';

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

describe('whether the shell is already explaining the silence', () => {
  it('leaves a view to explain itself while everything is answering', () => {
    const { result } = renderHook(() => useShellExplains());

    expect(result.current).toBe(false);
  });

  it('takes the explanation over when the feed dropped', () => {
    useFeedStore.getState().report('disconnected', 1);

    expect(renderHook(() => useShellExplains()).result.current).toBe(true);
  });

  it('leaves the first connect alone, before any retry', () => {
    useFeedStore.getState().report('connecting', 0);

    expect(renderHook(() => useShellExplains()).result.current).toBe(false);
  });

  it('takes the explanation over when the cluster stopped answering', () => {
    reportHealth('', { reachable: false, wobbling: false, reason: 'gone', cause: 'refused' });

    expect(renderHook(() => useShellExplains()).result.current).toBe(true);
  });

  it('takes the explanation over when the page belongs to an earlier run', () => {
    expireSession();

    expect(renderHook(() => useShellExplains()).result.current).toBe(true);
  });
});

import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ClusterBanner from '../../src/components/ClusterBanner';
import type { Health } from '../../src/store/clusterHealth';

function health(over: Partial<Health> = {}): Health {
  return {
    reachable: true,
    wobbling: false,
    reason: '',
    cause: '',
    since: 0,
    ...over,
  };
}

function show(over: Partial<Health>, onReconnect = vi.fn()) {
  render(<ClusterBanner cluster="p-mk2" health={health(over)} onReconnect={onReconnect} />);
  return onReconnect;
}

describe('the banner for a cluster that went quiet', () => {
  it('says nothing while the cluster answers', () => {
    show({});

    expect(screen.queryByRole('status')).toBeNull();
  });

  it('names the cluster, how long it has been quiet and what happened', () => {
    show({
      reachable: false,
      cause: 'timeout',
      reason: 'net/http: TLS handshake timeout',
      since: Date.now() - 42_000,
    });

    const banner = screen.getByRole('status');
    expect(banner).toHaveTextContent('p-mk2 stopped answering 42s ago');
    expect(banner).toHaveTextContent('What follows is the last it sent — timed out');
  });

  it('keeps the raw words the cluster used out of the way until asked', async () => {
    show({
      reachable: false,
      cause: 'timeout',
      reason: 'net/http: TLS handshake timeout',
      since: Date.now(),
    });

    expect(screen.queryByText('net/http: TLS handshake timeout')).toBeNull();

    await userEvent.click(screen.getByRole('button', { name: 'Details' }));

    expect(screen.getByText('net/http: TLS handshake timeout')).toBeVisible();
  });

  it('offers no details when the cluster gave no words', () => {
    show({ reachable: false, cause: 'other', reason: '', since: Date.now() });

    expect(screen.queryByRole('button', { name: 'Details' })).toBeNull();
  });

  it('reconnects the cluster on request', async () => {
    const onReconnect = show({
      reachable: false,
      cause: 'refused',
      reason: 'connection refused',
      since: Date.now(),
    });

    await userEvent.click(screen.getByRole('button', { name: 'Reconnect now' }));

    expect(onReconnect).toHaveBeenCalledOnce();
  });

  it('says a missed ping softly, without claiming the cluster is gone', () => {
    show({ reachable: true, wobbling: true, cause: 'timeout', reason: 'i/o timeout', since: 0 });

    const banner = screen.getByRole('status');
    expect(banner).toHaveTextContent('p-mk2 missed a ping');
    expect(banner).toHaveTextContent('Still showing what it last sent — timed out');
    expect(banner.className).toContain('warn');
    expect(banner.className).not.toContain('error');
  });

  it('still says what is on screen when the server named no cause', () => {
    show({ reachable: false, cause: '', reason: '', since: Date.now() });

    expect(screen.getByRole('status')).toHaveTextContent('What follows is the last it sent.');
  });

  it('says a missed ping with no cause without trailing punctuation of its own', () => {
    show({ reachable: true, wobbling: true, cause: '', reason: '', since: 0 });

    expect(screen.getByRole('status')).toHaveTextContent('Still showing what it last sent.');
  });

  it('still names the cluster when nobody recorded when it went quiet', () => {
    show({ reachable: false, cause: 'refused', reason: 'connection refused', since: 0 });

    expect(screen.getByRole('status')).toHaveTextContent('p-mk2 is not answering');
  });
});

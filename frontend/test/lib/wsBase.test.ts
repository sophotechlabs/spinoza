import { afterEach, describe, expect, it, vi } from 'vitest';
import { wsURL } from '../../src/lib/wsBase';

interface WsWindow {
  __SPINOZA_WS_BASE__?: string;
  __SPINOZA_TOKEN__?: string;
}

function clearOverride(): void {
  delete (window as unknown as WsWindow).__SPINOZA_WS_BASE__;
  delete (window as unknown as WsWindow).__SPINOZA_TOKEN__;
}

describe('wsURL', () => {
  afterEach(() => {
    clearOverride();
    vi.unstubAllGlobals();
  });

  it('carries the token the socket has no header to put it in', () => {
    (window as unknown as WsWindow).__SPINOZA_TOKEN__ = 's3cret';
    expect(wsURL('/ws')).toBe(`ws://${location.host}/ws?token=s3cret`);
  });

  it('appends the token to a path that already has a query', () => {
    (window as unknown as WsWindow).__SPINOZA_TOKEN__ = 's3c/ret';
    expect(wsURL('/api/exec?pod=web')).toBe(
      `ws://${location.host}/api/exec?pod=web&token=s3c%2Fret`,
    );
  });

  it('upgrades to wss on an https page', () => {
    vi.stubGlobal('location', { protocol: 'https:', host: 'spinoza.example' });
    expect(wsURL('/ws')).toBe('wss://spinoza.example/ws');
  });

  it('builds from the page location', () => {
    expect(wsURL('/ws')).toBe(`ws://${location.host}/ws`);
  });

  it('carries the query string', () => {
    expect(wsURL('/api/exec?pod=web')).toBe(`ws://${location.host}/api/exec?pod=web`);
  });

  it('honours the desktop override', () => {
    (window as unknown as WsWindow).__SPINOZA_WS_BASE__ = 'ws://127.0.0.1:51234';
    expect(wsURL('/ws')).toBe('ws://127.0.0.1:51234/ws');
  });
});

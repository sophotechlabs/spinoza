import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import type { PodRow } from '../../src/lib/types';
import { usePodsFeed } from '../../src/lib/feed';
import { usePodStore } from '../../src/store/pods';
import { makePod } from '../helpers';

class FakeWebSocket {
  static instances: FakeWebSocket[] = [];
  url: string;
  readyState = 0;
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  close = vi.fn((): void => {
    this.readyState = 3;
  });

  constructor(url: string) {
    this.url = url;
    FakeWebSocket.instances.push(this);
  }
}

interface WsWindow {
  __SPINOZA_WS_BASE__?: string;
}

function overrideBase(value: string): void {
  (window as unknown as WsWindow).__SPINOZA_WS_BASE__ = value;
}

function clearOverride(): void {
  delete (window as unknown as WsWindow).__SPINOZA_WS_BASE__;
}

function snapshotData(pod: PodRow): string {
  return JSON.stringify({ type: 'snapshot', resource: 'pods', items: [pod], rv: '1' });
}

function resetStore(): void {
  usePodStore.setState({ rows: new Map(), sorted: [] });
}

describe('usePodsFeed', () => {
  beforeEach(() => {
    FakeWebSocket.instances = [];
    vi.stubGlobal('WebSocket', FakeWebSocket);
    resetStore();
  });

  afterEach(() => {
    clearOverride();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
    vi.useRealTimers();
    resetStore();
  });

  it('connects to the same-origin /ws endpoint by default', () => {
    renderHook(() => usePodsFeed());
    expect(FakeWebSocket.instances).toHaveLength(1);
    expect(FakeWebSocket.instances[0].url).toBe(`ws://${location.host}/ws`);
  });

  it('uses the window override base when present', () => {
    overrideBase('ws://custom-host:9999');
    renderHook(() => usePodsFeed());
    expect(FakeWebSocket.instances[0].url).toBe('ws://custom-host:9999/ws');
  });

  it('upgrades to wss when the page is served over https', () => {
    vi.stubGlobal('location', { protocol: 'https:', host: 'secure.example' });
    renderHook(() => usePodsFeed());
    expect(FakeWebSocket.instances[0].url).toBe('wss://secure.example/ws');
  });

  it('starts in the connecting state', () => {
    const { result } = renderHook(() => usePodsFeed());
    expect(result.current.status).toBe('connecting');
  });

  it('reports connected after the socket opens', () => {
    const { result } = renderHook(() => usePodsFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      socket.onopen?.(new Event('open'));
    });
    expect(result.current.status).toBe('connected');
  });

  it('reports disconnected after the socket closes', () => {
    const { result } = renderHook(() => usePodsFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      socket.onclose?.(new CloseEvent('close'));
    });
    expect(result.current.status).toBe('disconnected');
  });

  it('closes the socket on error', () => {
    renderHook(() => usePodsFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      socket.onerror?.(new Event('error'));
    });
    expect(socket.close).toHaveBeenCalled();
  });

  it('applies a snapshot message to the store', () => {
    renderHook(() => usePodsFeed());
    const socket = FakeWebSocket.instances[0];
    const data = snapshotData(makePod({ uid: 'a', name: 'alpha' }));
    act(() => {
      socket.onmessage?.(new MessageEvent('message', { data }));
    });
    expect(usePodStore.getState().rows.get('a')?.name).toBe('alpha');
  });

  it('applies an added delta message to the store', () => {
    renderHook(() => usePodsFeed());
    const socket = FakeWebSocket.instances[0];
    const data = JSON.stringify({
      type: 'added',
      resource: 'pods',
      item: makePod({ uid: 'b', name: 'bravo' }),
    });
    act(() => {
      socket.onmessage?.(new MessageEvent('message', { data }));
    });
    expect(usePodStore.getState().rows.get('b')?.name).toBe('bravo');
  });

  it('applies a modified delta message to the store', () => {
    usePodStore.getState().applySnapshot([makePod({ uid: 'c', phase: 'Pending' })]);
    renderHook(() => usePodsFeed());
    const socket = FakeWebSocket.instances[0];
    const data = JSON.stringify({
      type: 'modified',
      resource: 'pods',
      item: makePod({ uid: 'c', phase: 'Running' }),
    });
    act(() => {
      socket.onmessage?.(new MessageEvent('message', { data }));
    });
    expect(usePodStore.getState().rows.get('c')?.phase).toBe('Running');
  });

  it('applies a deleted delta message to the store', () => {
    usePodStore.getState().applySnapshot([makePod({ uid: 'd', name: 'delta' })]);
    renderHook(() => usePodsFeed());
    const socket = FakeWebSocket.instances[0];
    const data = JSON.stringify({ type: 'deleted', resource: 'pods', uid: 'd' });
    act(() => {
      socket.onmessage?.(new MessageEvent('message', { data }));
    });
    expect(usePodStore.getState().rows.has('d')).toBe(false);
  });

  it('marks the feed disconnected on an error message', () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const { result } = renderHook(() => usePodsFeed());
    const socket = FakeWebSocket.instances[0];
    const data = JSON.stringify({ type: 'error', message: 'stream failed' });
    act(() => {
      socket.onmessage?.(new MessageEvent('message', { data }));
    });
    expect(result.current.status).toBe('disconnected');
    expect(errorSpy).toHaveBeenCalledWith('pods feed error:', 'stream failed');
  });

  it('ignores malformed json without throwing', () => {
    renderHook(() => usePodsFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      socket.onmessage?.(new MessageEvent('message', { data: 'not-json' }));
    });
    expect(usePodStore.getState().rows.size).toBe(0);
  });

  it('ignores messages with an unknown type', () => {
    renderHook(() => usePodsFeed());
    const socket = FakeWebSocket.instances[0];
    const data = JSON.stringify({ type: 'heartbeat' });
    act(() => {
      socket.onmessage?.(new MessageEvent('message', { data }));
    });
    expect(usePodStore.getState().rows.size).toBe(0);
  });

  it('reconnects on an exponential backoff schedule capped at 5000ms', () => {
    vi.useFakeTimers();
    renderHook(() => usePodsFeed());
    const delays = [500, 1000, 2000, 4000, 5000, 5000];
    let index = 0;
    while (index < delays.length) {
      const socket = FakeWebSocket.instances[index];
      act(() => {
        socket.onclose?.(new CloseEvent('close'));
      });
      const delay = delays[index];
      const before = FakeWebSocket.instances.length;
      act(() => {
        vi.advanceTimersByTime(delay - 1);
      });
      expect(FakeWebSocket.instances).toHaveLength(before);
      act(() => {
        vi.advanceTimersByTime(1);
      });
      expect(FakeWebSocket.instances).toHaveLength(before + 1);
      index += 1;
    }
  });

  it('resets the backoff after a successful reconnect', () => {
    vi.useFakeTimers();
    renderHook(() => usePodsFeed());
    act(() => {
      FakeWebSocket.instances[0].onclose?.(new CloseEvent('close'));
    });
    act(() => {
      vi.advanceTimersByTime(500);
    });
    act(() => {
      FakeWebSocket.instances[1].onopen?.(new Event('open'));
    });
    act(() => {
      FakeWebSocket.instances[1].onclose?.(new CloseEvent('close'));
    });
    const before = FakeWebSocket.instances.length;
    act(() => {
      vi.advanceTimersByTime(499);
    });
    expect(FakeWebSocket.instances).toHaveLength(before);
    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(FakeWebSocket.instances).toHaveLength(before + 1);
  });

  it('nulls the socket handlers and closes it on unmount', () => {
    const { unmount } = renderHook(() => usePodsFeed());
    const socket = FakeWebSocket.instances[0];
    unmount();
    expect(socket.close).toHaveBeenCalled();
    expect(socket.onopen).toBeNull();
    expect(socket.onmessage).toBeNull();
    expect(socket.onerror).toBeNull();
    expect(socket.onclose).toBeNull();
  });

  it('ignores socket events delivered after unmount', () => {
    const { unmount } = renderHook(() => usePodsFeed());
    const socket = FakeWebSocket.instances[0];
    const onopen = socket.onopen;
    const onmessage = socket.onmessage;
    const onclose = socket.onclose;
    unmount();
    onopen?.(new Event('open'));
    onmessage?.(new MessageEvent('message', { data: snapshotData(makePod({ uid: 'z' })) }));
    onclose?.(new CloseEvent('close'));
    expect(usePodStore.getState().rows.size).toBe(0);
    expect(FakeWebSocket.instances).toHaveLength(1);
  });

  it('does not reconnect when a retry timer fires after unmount', () => {
    const setTimeoutSpy = vi.spyOn(globalThis, 'setTimeout');
    const { unmount } = renderHook(() => usePodsFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      socket.onclose?.(new CloseEvent('close'));
    });
    const scheduled = setTimeoutSpy.mock.calls.find((call) => call[1] === 500);
    if (!scheduled) {
      throw new Error('reconnect was not scheduled');
    }
    const handler = scheduled[0];
    if (typeof handler !== 'function') {
      throw new Error('scheduled handler is not a function');
    }
    const reconnect = handler as () => void;
    const before = FakeWebSocket.instances.length;
    unmount();
    reconnect();
    expect(FakeWebSocket.instances).toHaveLength(before);
  });
});

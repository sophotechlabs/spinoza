import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import type { Row } from '../../src/lib/types';
import { useResourceFeed } from '../../src/lib/feed';
import { useResourcesStore } from '../../src/store/resources';
import { useLogsStore } from '../../src/store/logs';
import { makeColumns, makeDescriptor, makeRow } from '../helpers';

class FakeWebSocket {
  static instances: FakeWebSocket[] = [];
  url: string;
  readyState = 0;
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  send = vi.fn<(data: string) => void>();
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

function openSocket(socket: FakeWebSocket): void {
  socket.readyState = 1;
  socket.onopen?.(new Event('open'));
}

function resetStore(): void {
  useResourcesStore.setState({ subs: new Map() });
  useLogsStore.setState({ streams: new Map() });
}

const logRequest = {
  namespace: 'flux-system',
  name: 'web',
  container: 'app',
  tailLines: 500,
  follow: true,
};

function sentMessages(socket: FakeWebSocket): unknown[] {
  return socket.send.mock.calls.map((call) => JSON.parse(call[0]) as unknown);
}

function openFeedFor(subId: string): FakeWebSocket {
  const hook = renderHook(() => useResourceFeed());
  const socket = FakeWebSocket.instances[0];
  act(() => {
    openSocket(socket);
  });
  act(() => {
    hook.result.current.subscribe(subId, makeDescriptor({}), '');
  });
  return socket;
}

const descriptor = makeDescriptor({ group: 'apps', version: 'v1', resource: 'deployments' });

describe('useResourceFeed', () => {
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
    renderHook(() => useResourceFeed());
    expect(FakeWebSocket.instances).toHaveLength(1);
    expect(FakeWebSocket.instances[0].url).toBe(`ws://${location.host}/ws`);
  });

  it('uses the window override base when present', () => {
    overrideBase('ws://custom-host:9999');
    renderHook(() => useResourceFeed());
    expect(FakeWebSocket.instances[0].url).toBe('ws://custom-host:9999/ws');
  });

  it('upgrades to wss when the page is served over https', () => {
    vi.stubGlobal('location', { protocol: 'https:', host: 'secure.example' });
    renderHook(() => useResourceFeed());
    expect(FakeWebSocket.instances[0].url).toBe('wss://secure.example/ws');
  });

  it('starts in the connecting state', () => {
    const { result } = renderHook(() => useResourceFeed());
    expect(result.current.status).toBe('connecting');
  });

  it('reports connected after the socket opens', () => {
    const { result } = renderHook(() => useResourceFeed());
    act(() => {
      openSocket(FakeWebSocket.instances[0]);
    });
    expect(result.current.status).toBe('connected');
  });

  it('reports disconnected after the socket closes', () => {
    const { result } = renderHook(() => useResourceFeed());
    act(() => {
      FakeWebSocket.instances[0].onclose?.(new CloseEvent('close'));
    });
    expect(result.current.status).toBe('disconnected');
  });

  it('closes the socket on error', () => {
    renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      socket.onerror?.(new Event('error'));
    });
    expect(socket.close).toHaveBeenCalled();
  });

  it('routes a snapshot message to the store by subId', () => {
    const socket = openFeedFor('main');
    const data = JSON.stringify({
      type: 'snapshot',
      subId: 'main',
      columns: makeColumns(['Ready']),
      namespaced: true,
      rows: [makeRow({ uid: 'a', name: 'alpha' })],
    });
    act(() => {
      socket.onmessage?.(new MessageEvent('message', { data }));
    });
    expect(useResourcesStore.getState().subs.get('main')?.rows.get('a')?.name).toBe('alpha');
  });

  it('survives a snapshot that carries no rows key', () => {
    const socket = openFeedFor('main');
    const data = JSON.stringify({
      type: 'snapshot',
      subId: 'main',
      columns: makeColumns(['Ready']),
      namespaced: true,
    });
    act(() => {
      socket.onmessage?.(new MessageEvent('message', { data }));
    });

    const sub = useResourcesStore.getState().subs.get('main');
    expect(sub).toBeDefined();
    expect(sub?.rows.size).toBe(0);
  });

  it('still applies deltas after an empty snapshot', () => {
    const socket = openFeedFor('main');
    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({ type: 'snapshot', subId: 'main', columns: makeColumns([]) }),
        }),
      );
    });
    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({
            type: 'added',
            subId: 'main',
            row: makeRow({ uid: 'z', name: 'zulu' }),
          }),
        }),
      );
    });

    expect(useResourcesStore.getState().subs.get('main')?.rows.get('z')?.name).toBe('zulu');
  });

  it('treats a missing namespaced flag as cluster scoped', () => {
    const socket = openFeedFor('main');
    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({ type: 'snapshot', subId: 'main', columns: [], rows: [] }),
        }),
      );
    });

    expect(useResourcesStore.getState().subs.get('main')?.namespaced).toBe(false);
  });

  it('ignores a snapshot for a subscription that was already dropped', () => {
    const hook = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
    });
    act(() => {
      hook.result.current.subscribe('main#1', makeDescriptor({}), '');
    });
    act(() => {
      hook.result.current.unsubscribe('main#1');
      hook.result.current.subscribe('main#2', makeDescriptor({}), '');
    });

    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({
            type: 'snapshot',
            subId: 'main#1',
            columns: makeColumns([]),
            namespaced: true,
            rows: [makeRow({ uid: 'stale', name: 'from-the-old-resource' })],
          }),
        }),
      );
    });

    expect(useResourcesStore.getState().subs.has('main#1')).toBe(false);
  });

  it('ignores a delta for a subscription that was already dropped', () => {
    const hook = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
    });
    act(() => {
      hook.result.current.subscribe('main#1', makeDescriptor({}), '');
    });
    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({
            type: 'snapshot',
            subId: 'main#1',
            columns: makeColumns([]),
            namespaced: true,
            rows: [],
          }),
        }),
      );
    });
    act(() => {
      hook.result.current.unsubscribe('main#1');
    });

    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({
            type: 'added',
            subId: 'main#1',
            row: makeRow({ uid: 'ghost', name: 'ghost' }),
          }),
        }),
      );
    });

    expect(useResourcesStore.getState().subs.has('main#1')).toBe(false);
  });

  it('ignores log lines for a stream that was already stopped', () => {
    const hook = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
    });
    act(() => {
      hook.result.current.subscribeLogs('logs#1', logRequest);
    });
    act(() => {
      hook.result.current.unsubscribeLogs('logs#1');
    });

    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({ type: 'log', subId: 'logs#1', lines: ['late'] }),
        }),
      );
    });

    expect(useLogsStore.getState().streams.has('logs#1')).toBe(false);
  });

  it('drops an error for a subscription nobody holds any more', () => {
    const hook = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
    });
    act(() => {
      hook.result.current.subscribe('main#3', makeDescriptor({}), '');
    });
    act(() => {
      hook.result.current.unsubscribe('main#3');
    });

    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({ type: 'error', subId: 'main#3', message: 'too late' }),
        }),
      );
    });

    expect(useResourcesStore.getState().errors.has('main#3')).toBe(false);
  });

  it('keeps a table error out of the log store', () => {
    const socket = openFeedFor('main');
    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({ type: 'error', subId: 'main', message: 'watch failed' }),
        }),
      );
    });

    expect(useResourcesStore.getState().errors.get('main')).toBe('watch failed');
    expect(useLogsStore.getState().streams.has('main')).toBe(false);
  });

  it('shows a failed subscription instead of an empty table', () => {
    const socket = openFeedFor('main');
    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({
            type: 'error',
            subId: 'main',
            message: 'pods is forbidden: User "spinoza" cannot list resource "pods"',
          }),
        }),
      );
    });

    expect(useResourcesStore.getState().errors.get('main')).toContain('is forbidden');
  });

  it('clears the error once a snapshot arrives', () => {
    const socket = openFeedFor('main');
    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({ type: 'error', subId: 'main', message: 'boom' }),
        }),
      );
    });
    expect(useResourcesStore.getState().errors.get('main')).toBe('boom');

    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({
            type: 'snapshot',
            subId: 'main',
            columns: makeColumns([]),
            namespaced: true,
            rows: [],
          }),
        }),
      );
    });

    expect(useResourcesStore.getState().errors.has('main')).toBe(false);
  });

  it('marks a log stream as failed rather than silently ending it', () => {
    const hook = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
    });
    act(() => {
      hook.result.current.subscribeLogs('logs#1', logRequest);
    });

    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({
            type: 'error',
            subId: 'logs#1',
            message: 'pods/log is forbidden',
          }),
        }),
      );
    });

    expect(useLogsStore.getState().streams.get('logs#1')?.error).toBe('pods/log is forbidden');
    expect(useResourcesStore.getState().errors.has('logs#1')).toBe(false);
  });

  it('routes an added delta message to the store', () => {
    useResourcesStore.getState().applySnapshot('main', makeColumns([]), true, []);
    const socket = openFeedFor('main');
    const row: Row = makeRow({ uid: 'b', name: 'bravo' });
    const data = JSON.stringify({ type: 'added', subId: 'main', row });
    act(() => {
      socket.onmessage?.(new MessageEvent('message', { data }));
    });
    expect(useResourcesStore.getState().subs.get('main')?.rows.get('b')?.name).toBe('bravo');
  });

  it('routes a modified delta message to the store', () => {
    useResourcesStore
      .getState()
      .applySnapshot('main', makeColumns([]), true, [makeRow({ uid: 'c', name: 'before' })]);
    const socket = openFeedFor('main');
    const data = JSON.stringify({
      type: 'modified',
      subId: 'main',
      row: makeRow({ uid: 'c', name: 'after' }),
    });
    act(() => {
      socket.onmessage?.(new MessageEvent('message', { data }));
    });
    expect(useResourcesStore.getState().subs.get('main')?.rows.get('c')?.name).toBe('after');
  });

  it('routes a deleted delta message to the store', () => {
    useResourcesStore
      .getState()
      .applySnapshot('main', makeColumns([]), true, [makeRow({ uid: 'd' })]);
    const socket = openFeedFor('main');
    const data = JSON.stringify({ type: 'deleted', subId: 'main', uid: 'd' });
    act(() => {
      socket.onmessage?.(new MessageEvent('message', { data }));
    });
    expect(useResourcesStore.getState().subs.get('main')?.rows.has('d')).toBe(false);
  });

  it('records an error message without changing connection state', () => {
    const hook = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
    });
    act(() => {
      hook.result.current.subscribe('main', makeDescriptor({}), '');
    });
    const data = JSON.stringify({ type: 'error', subId: 'main', message: 'stream failed' });
    act(() => {
      socket.onmessage?.(new MessageEvent('message', { data }));
    });

    expect(hook.result.current.status).toBe('connected');
    expect(useResourcesStore.getState().errors.get('main')).toBe('stream failed');
  });

  it('ignores malformed json without throwing', () => {
    renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      socket.onmessage?.(new MessageEvent('message', { data: 'not-json' }));
    });
    expect(useResourcesStore.getState().subs.size).toBe(0);
  });

  it('ignores messages with an unknown type', () => {
    renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    const data = JSON.stringify({ type: 'heartbeat', subId: 'main' });
    act(() => {
      socket.onmessage?.(new MessageEvent('message', { data }));
    });
    expect(useResourcesStore.getState().subs.size).toBe(0);
  });

  it('sends a subscribe frame immediately when the socket is open', () => {
    const { result } = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
    });
    act(() => {
      result.current.subscribe('main', descriptor, 'prod');
    });
    expect(sentMessages(socket)).toEqual([
      {
        type: 'subscribe',
        subId: 'main',
        group: 'apps',
        version: 'v1',
        resource: 'deployments',
        namespace: 'prod',
      },
    ]);
  });

  it('queues a subscription while closed and flushes it on open', () => {
    const { result } = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      result.current.subscribe('main', descriptor, '');
    });
    expect(socket.send).not.toHaveBeenCalled();
    act(() => {
      openSocket(socket);
    });
    expect(sentMessages(socket)).toEqual([
      {
        type: 'subscribe',
        subId: 'main',
        group: 'apps',
        version: 'v1',
        resource: 'deployments',
        namespace: '',
      },
    ]);
  });

  it('re-sends active subscriptions after a reconnect', () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useResourceFeed());
    const first = FakeWebSocket.instances[0];
    act(() => {
      openSocket(first);
    });
    act(() => {
      result.current.subscribe('main', descriptor, 'prod');
    });
    act(() => {
      first.onclose?.(new CloseEvent('close'));
    });
    act(() => {
      vi.advanceTimersByTime(500);
    });
    const second = FakeWebSocket.instances[1];
    act(() => {
      openSocket(second);
    });
    expect(sentMessages(second)).toEqual([
      {
        type: 'subscribe',
        subId: 'main',
        group: 'apps',
        version: 'v1',
        resource: 'deployments',
        namespace: 'prod',
      },
    ]);
  });

  it('sends an unsubscribe frame and clears the sub from the store', () => {
    useResourcesStore
      .getState()
      .applySnapshot('main', makeColumns([]), true, [makeRow({ uid: 'a' })]);
    const { result } = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
    });
    act(() => {
      result.current.subscribe('main', descriptor, '');
    });
    socket.send.mockClear();
    act(() => {
      result.current.unsubscribe('main');
    });
    expect(sentMessages(socket)).toEqual([{ type: 'unsubscribe', subId: 'main' }]);
    expect(useResourcesStore.getState().subs.has('main')).toBe(false);
  });

  it('does not re-send an unsubscribed subscription after a reconnect', () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useResourceFeed());
    const first = FakeWebSocket.instances[0];
    act(() => {
      openSocket(first);
    });
    act(() => {
      result.current.subscribe('main', descriptor, '');
    });
    act(() => {
      result.current.unsubscribe('main');
    });
    act(() => {
      first.onclose?.(new CloseEvent('close'));
    });
    act(() => {
      vi.advanceTimersByTime(500);
    });
    const second = FakeWebSocket.instances[1];
    act(() => {
      openSocket(second);
    });
    expect(second.send).not.toHaveBeenCalled();
  });

  it('does not throw when subscribe is called after unmount', () => {
    const { result, unmount } = renderHook(() => useResourceFeed());
    const subscribe = result.current.subscribe;
    unmount();
    expect(() => {
      subscribe('main', descriptor, '');
    }).not.toThrow();
  });

  it('reconnect closes the current socket and opens a new one immediately', () => {
    const { result } = renderHook(() => useResourceFeed());
    const first = FakeWebSocket.instances[0];
    act(() => {
      openSocket(first);
    });
    act(() => {
      result.current.reconnect();
    });
    expect(first.close).toHaveBeenCalled();
    expect(first.onclose).toBeNull();
    expect(FakeWebSocket.instances).toHaveLength(2);
  });

  it('does nothing when reconnect is called after unmount', () => {
    const { result, unmount } = renderHook(() => useResourceFeed());
    const reconnect = result.current.reconnect;
    unmount();
    expect(() => {
      reconnect();
    }).not.toThrow();
    expect(FakeWebSocket.instances).toHaveLength(1);
  });

  it('reconnect cancels a pending backoff timer', () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useResourceFeed());
    act(() => {
      FakeWebSocket.instances[0].onclose?.(new CloseEvent('close'));
    });
    act(() => {
      result.current.reconnect();
    });
    expect(FakeWebSocket.instances).toHaveLength(2);
    act(() => {
      vi.advanceTimersByTime(5000);
    });
    expect(FakeWebSocket.instances).toHaveLength(2);
  });

  it('reconnects on an exponential backoff schedule capped at 5000ms', () => {
    vi.useFakeTimers();
    renderHook(() => useResourceFeed());
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
    renderHook(() => useResourceFeed());
    act(() => {
      FakeWebSocket.instances[0].onclose?.(new CloseEvent('close'));
    });
    act(() => {
      vi.advanceTimersByTime(500);
    });
    act(() => {
      openSocket(FakeWebSocket.instances[1]);
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
    const { unmount } = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    unmount();
    expect(socket.close).toHaveBeenCalled();
    expect(socket.onopen).toBeNull();
    expect(socket.onmessage).toBeNull();
    expect(socket.onerror).toBeNull();
    expect(socket.onclose).toBeNull();
  });

  it('ignores socket events delivered after unmount', () => {
    const { unmount } = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    const onopen = socket.onopen;
    const onmessage = socket.onmessage;
    const onclose = socket.onclose;
    unmount();
    const data = JSON.stringify({
      type: 'snapshot',
      subId: 'main',
      columns: [],
      namespaced: true,
      rows: [makeRow({ uid: 'z' })],
    });
    onopen?.(new Event('open'));
    onmessage?.(new MessageEvent('message', { data }));
    onclose?.(new CloseEvent('close'));
    expect(useResourcesStore.getState().subs.size).toBe(0);
    expect(FakeWebSocket.instances).toHaveLength(1);
  });

  it('does not reconnect when a retry timer fires after unmount', () => {
    const setTimeoutSpy = vi.spyOn(globalThis, 'setTimeout');
    const { unmount } = renderHook(() => useResourceFeed());
    act(() => {
      FakeWebSocket.instances[0].onclose?.(new CloseEvent('close'));
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
  it('sends a logs-subscribe frame and opens a stream', () => {
    const { result } = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
    });
    act(() => {
      result.current.subscribeLogs('logs', logRequest);
    });

    expect(sentMessages(socket)).toContainEqual({
      type: 'logs-subscribe',
      subId: 'logs',
      ...logRequest,
    });
    expect(useLogsStore.getState().streams.get('logs')).toEqual({
      lines: [],
      dropped: 0,
      ended: false,
    });
  });

  it('queues a logs subscription while closed and flushes it on open', () => {
    const { result } = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      result.current.subscribeLogs('logs', logRequest);
    });

    expect(socket.send).not.toHaveBeenCalled();

    act(() => {
      openSocket(socket);
    });

    expect(sentMessages(socket)).toContainEqual({
      type: 'logs-subscribe',
      subId: 'logs',
      ...logRequest,
    });
  });

  it('appends streamed log lines to the store', () => {
    const { result } = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
      result.current.subscribeLogs('logs', logRequest);
    });
    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({ type: 'log', subId: 'logs', lines: ['first', 'second'] }),
        }),
      );
    });

    expect(useLogsStore.getState().streams.get('logs')?.lines).toEqual(['first', 'second']);
  });

  it('marks a stream ended on log-end', () => {
    const { result } = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
      result.current.subscribeLogs('logs', logRequest);
    });
    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({ type: 'log-end', subId: 'logs' }),
        }),
      );
    });

    expect(useLogsStore.getState().streams.get('logs')?.ended).toBe(true);
  });

  it('sends a logs-unsubscribe frame and frees the buffer', () => {
    const { result } = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
      result.current.subscribeLogs('logs', logRequest);
    });
    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({ type: 'log', subId: 'logs', lines: ['first'] }),
        }),
      );
    });
    act(() => {
      result.current.unsubscribeLogs('logs');
    });

    expect(sentMessages(socket)).toContainEqual({ type: 'logs-unsubscribe', subId: 'logs' });
    expect(useLogsStore.getState().streams.has('logs')).toBe(false);
  });

  it('re-sends an active log subscription after a reconnect', () => {
    const { result } = renderHook(() => useResourceFeed());
    const first = FakeWebSocket.instances[0];
    act(() => {
      openSocket(first);
      result.current.subscribeLogs('logs', logRequest);
    });
    act(() => {
      first.onclose?.(new CloseEvent('close'));
    });
    act(() => {
      result.current.reconnect();
    });
    const second = FakeWebSocket.instances[FakeWebSocket.instances.length - 1];
    act(() => {
      openSocket(second);
    });

    expect(sentMessages(second)).toContainEqual({
      type: 'logs-subscribe',
      subId: 'logs',
      ...logRequest,
    });
  });

  it('does not re-send an unsubscribed log stream after a reconnect', () => {
    const { result } = renderHook(() => useResourceFeed());
    const first = FakeWebSocket.instances[0];
    act(() => {
      openSocket(first);
      result.current.subscribeLogs('logs', logRequest);
      result.current.unsubscribeLogs('logs');
    });
    act(() => {
      result.current.reconnect();
    });
    const second = FakeWebSocket.instances[FakeWebSocket.instances.length - 1];
    act(() => {
      openSocket(second);
    });

    expect(sentMessages(second)).not.toContainEqual(
      expect.objectContaining({ type: 'logs-subscribe' }),
    );
  });

  it('does not throw when log helpers are called after unmount', () => {
    const { result, unmount } = renderHook(() => useResourceFeed());
    act(() => {
      openSocket(FakeWebSocket.instances[0]);
    });
    unmount();

    expect(() => {
      result.current.subscribeLogs('logs', logRequest);
      result.current.unsubscribeLogs('logs');
    }).not.toThrow();
  });
});

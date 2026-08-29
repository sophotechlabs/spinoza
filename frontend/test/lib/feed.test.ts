import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import type { Row } from '../../src/lib/types';
import { expireSession } from '../../src/store/session';
import { DELTA_FLUSH_MS, useResourceFeed } from '../../src/lib/feed';
import { useResourcesStore } from '../../src/store/resources';
import { useLogsStore } from '../../src/store/logs';
import { useContextsStore } from '../../src/store/contexts';
import { useClusterHealthStore } from '../../src/store/clusterHealth';
import { makeColumns, makeDescriptor, makeRow } from '../helpers';
import { setActiveCluster } from '../../src/lib/cluster';

const mk1 = 'https://p-mk1:6443';

const mk2 = 'https://p-mk2:6443';

async function flushDeltas(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, DELTA_FLUSH_MS + 5));
  });
}

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
    hook.result.current.subscribe(subId, makeDescriptor({}), '', []);
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
    expect(FakeWebSocket.instances[0].url).toBe(`ws://${location.host}/ws?view=browser`);
  });

  it('uses the window override base when present', () => {
    overrideBase('ws://custom-host:9999');
    renderHook(() => useResourceFeed());
    expect(FakeWebSocket.instances[0].url).toBe('ws://custom-host:9999/ws?view=browser');
  });

  it('upgrades to wss when the page is served over https', () => {
    vi.stubGlobal('location', { protocol: 'https:', host: 'secure.example' });
    renderHook(() => useResourceFeed());
    expect(FakeWebSocket.instances[0].url).toBe('wss://secure.example/ws?view=browser');
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

  it('counts the retries while the socket stays down', async () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useResourceFeed());
    expect(result.current.attempt).toBe(0);

    act(() => {
      FakeWebSocket.instances[0].onclose?.(new CloseEvent('close'));
    });
    expect(result.current.attempt).toBe(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(600);
    });
    act(() => {
      FakeWebSocket.instances[1].onclose?.(new CloseEvent('close'));
    });

    expect(result.current.attempt).toBe(2);
    vi.useRealTimers();
  });

  it('goes back to zero retries once the socket opens', () => {
    const { result } = renderHook(() => useResourceFeed());
    act(() => {
      FakeWebSocket.instances[0].onclose?.(new CloseEvent('close'));
    });
    expect(result.current.attempt).toBe(1);

    act(() => {
      openSocket(FakeWebSocket.instances[0]);
    });

    expect(result.current.attempt).toBe(0);
  });

  it('goes back to zero retries when the user reconnects by hand', () => {
    const { result } = renderHook(() => useResourceFeed());
    act(() => {
      FakeWebSocket.instances[0].onclose?.(new CloseEvent('close'));
    });

    act(() => {
      result.current.reconnect();
    });

    expect(result.current.attempt).toBe(0);
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

  it('still applies deltas after an empty snapshot', async () => {
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

    await flushDeltas();

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

  it('ignores a snapshot for a subscription that was already dropped', async () => {
    const hook = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
    });
    act(() => {
      hook.result.current.subscribe('main#1', makeDescriptor({}), '', []);
    });
    act(() => {
      hook.result.current.unsubscribe('main#1');
      hook.result.current.subscribe('main#2', makeDescriptor({}), '', []);
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

    await flushDeltas();

    expect(useResourcesStore.getState().subs.has('main#1')).toBe(false);
  });

  it('ignores a delta for a subscription that was already dropped', async () => {
    const hook = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
    });
    act(() => {
      hook.result.current.subscribe('main#1', makeDescriptor({}), '', []);
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

    await flushDeltas();

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
      hook.result.current.subscribe('main#3', makeDescriptor({}), '', []);
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

  it('routes an added delta message to the store', async () => {
    useResourcesStore.getState().applySnapshot('main', makeColumns([]), true, []);
    const socket = openFeedFor('main');
    const row: Row = makeRow({ uid: 'b', name: 'bravo' });
    const data = JSON.stringify({ type: 'added', subId: 'main', row });
    act(() => {
      socket.onmessage?.(new MessageEvent('message', { data }));
    });
    await flushDeltas();
    expect(useResourcesStore.getState().subs.get('main')?.rows.get('b')?.name).toBe('bravo');
  });

  it('routes a modified delta message to the store', async () => {
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
    await flushDeltas();
    expect(useResourcesStore.getState().subs.get('main')?.rows.get('c')?.name).toBe('after');
  });

  it('routes a deleted delta message to the store', async () => {
    useResourcesStore
      .getState()
      .applySnapshot('main', makeColumns([]), true, [makeRow({ uid: 'd' })]);
    const socket = openFeedFor('main');
    const data = JSON.stringify({ type: 'deleted', subId: 'main', uid: 'd' });
    act(() => {
      socket.onmessage?.(new MessageEvent('message', { data }));
    });
    await flushDeltas();
    expect(useResourcesStore.getState().subs.get('main')?.rows.has('d')).toBe(false);
  });

  it('records an error message without changing connection state', () => {
    const hook = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
    });
    act(() => {
      hook.result.current.subscribe('main', makeDescriptor({}), '', []);
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
      result.current.subscribe('main', descriptor, 'prod', []);
    });
    expect(sentMessages(socket)).toEqual([
      {
        type: 'subscribe',
        subId: 'main',
        group: 'apps',
        version: 'v1',
        resource: 'deployments',
        namespace: 'prod',
        limit: 0,
        filters: [],
      },
    ]);
  });

  it('queues a subscription while closed and flushes it on open', () => {
    const { result } = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      result.current.subscribe('main', descriptor, '', []);
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
        limit: 0,
        filters: [],
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
      result.current.subscribe('main', descriptor, 'prod', []);
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
        limit: 0,
        filters: [],
      },
    ]);
  });

  it('asks for a wider window and remembers it across a reconnect', () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useResourceFeed());
    const first = FakeWebSocket.instances[0];
    act(() => {
      openSocket(first);
    });
    act(() => {
      result.current.subscribe('main', descriptor, 'prod', []);
    });
    act(() => {
      result.current.loadMore('main', 200);
    });

    expect(sentMessages(first)[1]).toEqual({ type: 'more', subId: 'main', limit: 200 });

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

    expect(sentMessages(second)[0]).toMatchObject({ type: 'subscribe', limit: 200 });
  });

  it('has nothing to widen for a subscription it does not hold', () => {
    const { result } = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
    });

    act(() => {
      result.current.loadMore('missing', 200);
    });

    expect(sentMessages(socket)).toEqual([]);
  });

  it('remembers a wider window asked for while the socket is closed', () => {
    const { result } = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      result.current.subscribe('main', descriptor, 'prod', []);
    });
    act(() => {
      result.current.loadMore('main', 300);
    });
    act(() => {
      openSocket(socket);
    });

    expect(sentMessages(socket)[0]).toMatchObject({ type: 'subscribe', limit: 300 });
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
      result.current.subscribe('main', descriptor, '', []);
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
      result.current.subscribe('main', descriptor, '', []);
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
      subscribe('main', descriptor, '', []);
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
      sources: [],
      dropped: 0,
      revision: 0,
      ended: false,
      resumed: false,
      attached: 0,
      matched: 0,
      opened: false,
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

  it('appends streamed log lines to the store', async () => {
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

    await flushDeltas();

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

  it('keeps the lines already on screen through a reconnect', async () => {
    const { result } = renderHook(() => useResourceFeed());
    const first = FakeWebSocket.instances[0];
    act(() => {
      openSocket(first);
      result.current.subscribeLogs('logs', logRequest);
    });
    act(() => {
      first.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({ type: 'log', subId: 'logs', lines: ['before the drop'] }),
        }),
      );
    });
    await flushDeltas();
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

    const stream = useLogsStore.getState().streams.get('logs');
    expect(stream?.lines).toEqual(['before the drop']);
    expect(stream?.resumed).toBe(true);
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
  it('sends the chips so the server can look past the window', () => {
    const { result } = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
    });

    act(() => {
      result.current.subscribe('main', descriptor, 'prod', [{ field: 'type', value: 'Warning' }]);
    });

    expect(sentMessages(socket)).toEqual([
      {
        type: 'subscribe',
        subId: 'main',
        group: 'apps',
        version: 'v1',
        resource: 'deployments',
        namespace: 'prod',
        limit: 0,
        filters: [{ field: 'type', value: 'Warning' }],
      },
    ]);
  });

  it('keeps the chips when the socket comes back', () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useResourceFeed());
    const first = FakeWebSocket.instances[0];
    act(() => {
      openSocket(first);
    });
    act(() => {
      result.current.subscribe('main', descriptor, 'prod', [{ field: 'type', value: 'Warning' }]);
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

    const resent = sentMessages(second)[0] as { filters: unknown };
    expect(resent.filters).toEqual([{ field: 'type', value: 'Warning' }]);
  });
});

describe('a burst of watch events', () => {
  beforeEach(() => {
    useResourcesStore.setState({ subs: new Map(), errors: new Map() });
    FakeWebSocket.instances = [];
    vi.stubGlobal('WebSocket', FakeWebSocket);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    useResourcesStore.setState({ subs: new Map(), errors: new Map() });
  });

  function send(socket: FakeWebSocket, msg: unknown): void {
    act(() => {
      socket.onmessage?.(new MessageEvent('message', { data: JSON.stringify(msg) }));
    });
  }

  it('lands as a single store write instead of one per event', async () => {
    useResourcesStore.getState().applySnapshot('main', makeColumns([]), true, []);
    const socket = openFeedFor('main');
    let writes = 0;
    const stop = useResourcesStore.subscribe(() => {
      writes += 1;
    });

    for (let index = 0; index < 50; index += 1) {
      send(socket, {
        type: 'added',
        subId: 'main',
        row: makeRow({ uid: `u-${String(index)}`, name: `pod-${String(index)}` }),
      });
    }
    expect(writes).toBe(0);

    await flushDeltas();
    stop();

    expect(writes).toBe(1);
    expect(useResourcesStore.getState().subs.get('main')?.rows.size).toBe(50);
  });

  it('keeps the last value when the same object changes twice in one window', async () => {
    useResourcesStore.getState().applySnapshot('main', makeColumns([]), true, []);
    const socket = openFeedFor('main');

    send(socket, { type: 'added', subId: 'main', row: makeRow({ uid: 'a', name: 'first' }) });
    send(socket, { type: 'modified', subId: 'main', row: makeRow({ uid: 'a', name: 'second' }) });
    await flushDeltas();

    expect(useResourcesStore.getState().subs.get('main')?.rows.get('a')?.name).toBe('second');
  });

  it('drops buffered deltas when a fresh snapshot supersedes them', async () => {
    useResourcesStore.getState().applySnapshot('main', makeColumns([]), true, []);
    const socket = openFeedFor('main');

    send(socket, { type: 'added', subId: 'main', row: makeRow({ uid: 'gone', name: 'gone' }) });
    send(socket, {
      type: 'snapshot',
      subId: 'main',
      columns: makeColumns([]),
      namespaced: true,
      rows: [makeRow({ uid: 'fresh', name: 'fresh' })],
    });
    await flushDeltas();

    const rows = useResourcesStore.getState().subs.get('main')?.rows;
    expect(rows?.has('gone')).toBe(false);
    expect(rows?.has('fresh')).toBe(true);
  });

  it('does not write at all when the only delta deletes a row that was never there', async () => {
    useResourcesStore.getState().applySnapshot('main', makeColumns([]), true, []);
    const socket = openFeedFor('main');
    let writes = 0;
    const stop = useResourcesStore.subscribe(() => {
      writes += 1;
    });

    send(socket, { type: 'deleted', subId: 'main', uid: 'never-existed' });
    await flushDeltas();
    stop();

    expect(writes).toBe(0);
  });
});

describe('deltas still waiting when the feed goes away', () => {
  beforeEach(() => {
    useResourcesStore.setState({ subs: new Map(), errors: new Map() });
    FakeWebSocket.instances = [];
    vi.stubGlobal('WebSocket', FakeWebSocket);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    useResourcesStore.setState({ subs: new Map(), errors: new Map() });
  });

  it('are dropped rather than written after unmount', async () => {
    useResourcesStore.getState().applySnapshot('main', makeColumns([]), true, []);
    const hook = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
    });
    act(() => {
      hook.result.current.subscribe('main', makeDescriptor({}), '', []);
    });
    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({
            type: 'added',
            subId: 'main',
            row: makeRow({ uid: 'late', name: 'late' }),
          }),
        }),
      );
    });

    hook.unmount();
    await flushDeltas();

    expect(useResourcesStore.getState().subs.get('main')?.rows.has('late')).toBe(false);
  });

  it('are dropped when the feed reconnects before the window closes', async () => {
    useResourcesStore.getState().applySnapshot('main', makeColumns([]), true, []);
    const hook = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
    });
    act(() => {
      hook.result.current.subscribe('main', makeDescriptor({}), '', []);
    });
    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({
            type: 'added',
            subId: 'main',
            row: makeRow({ uid: 'late', name: 'late' }),
          }),
        }),
      );
    });

    act(() => {
      hook.result.current.reconnect();
    });
    await flushDeltas();

    expect(useResourcesStore.getState().subs.get('main')?.rows.has('late')).toBe(false);
  });
});

describe('a chatty log stream', () => {
  beforeEach(() => {
    useLogsStore.setState({ streams: new Map() });
    FakeWebSocket.instances = [];
    vi.stubGlobal('WebSocket', FakeWebSocket);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    useLogsStore.setState({ streams: new Map() });
  });

  function openLogFeed() {
    const hook = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
    });
    act(() => {
      hook.result.current.subscribeLogs('logs', logRequest);
    });
    return { hook, socket };
  }

  function sendLines(socket: FakeWebSocket, lines: string[]): void {
    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({ type: 'log', subId: 'logs', lines }),
        }),
      );
    });
  }

  it('lands as one append instead of one per message', async () => {
    const { socket } = openLogFeed();
    let writes = 0;
    const stop = useLogsStore.subscribe(() => {
      writes += 1;
    });

    for (let index = 0; index < 20; index += 1) {
      sendLines(socket, [`line ${String(index)}`]);
    }
    expect(writes).toBe(0);

    await flushDeltas();
    stop();

    expect(writes).toBe(1);
    expect(useLogsStore.getState().streams.get('logs')?.lines).toHaveLength(20);
  });

  function sendFrom(socket: FakeWebSocket, source: string, lines: string[]): void {
    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({ type: 'log', subId: 'logs', lines, source }),
        }),
      );
    });
  }

  it('keeps every batch on the pod that wrote it', async () => {
    const { socket } = openLogFeed();

    sendFrom(socket, 'web-0', ['one']);
    sendFrom(socket, 'web-1', ['two']);
    sendFrom(socket, 'web-0', ['three']);
    await flushDeltas();

    const stream = useLogsStore.getState().streams.get('logs');
    expect(stream?.lines).toEqual(['one', 'two', 'three']);
    expect(stream?.sources).toEqual(['web-0', 'web-1', 'web-0']);
  });

  it('writes once per pod rather than once per message', async () => {
    const { socket } = openLogFeed();
    let writes = 0;
    const stop = useLogsStore.subscribe(() => {
      writes += 1;
    });

    sendFrom(socket, 'web-0', ['one']);
    sendFrom(socket, 'web-0', ['two']);
    sendFrom(socket, 'web-1', ['three']);
    await flushDeltas();
    stop();

    expect(writes).toBe(2);
  });

  it('takes the pod count the server reports while the stream runs', () => {
    const { socket } = openLogFeed();

    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({ type: 'log-open', subId: 'logs', attached: 3, matched: 7 }),
        }),
      );
    });

    expect(useLogsStore.getState().streams.get('logs')).toMatchObject({
      attached: 3,
      matched: 7,
    });
  });

  it('does not lose the tail when the stream ends inside the window', () => {
    const { socket } = openLogFeed();

    sendLines(socket, ['last words']);
    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({ type: 'log-end', subId: 'logs' }),
        }),
      );
    });

    const stream = useLogsStore.getState().streams.get('logs');
    expect(stream?.lines).toEqual(['last words']);
    expect(stream?.ended).toBe(true);
  });

  it('does not lose the tail when the stream fails inside the window', () => {
    const { socket } = openLogFeed();

    sendLines(socket, ['before the error']);
    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({ type: 'error', subId: 'logs', message: 'forbidden' }),
        }),
      );
    });

    const stream = useLogsStore.getState().streams.get('logs');
    expect(stream?.lines).toEqual(['before the error']);
    expect(stream?.error).toBe('forbidden');
  });
});

describe('the socket on a page whose token died', () => {
  it('gives up reconnecting, because a new socket would be refused too', async () => {
    vi.useFakeTimers();
    renderHook(() => useResourceFeed());
    const opened = FakeWebSocket.instances.length;

    expireSession();
    act(() => {
      FakeWebSocket.instances[opened - 1].onclose?.(new CloseEvent('close'));
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30000);
    });

    expect(FakeWebSocket.instances).toHaveLength(opened);
    vi.useRealTimers();
  });
});

describe('a batch of row changes', () => {
  beforeEach(() => {
    FakeWebSocket.instances = [];
    vi.stubGlobal('WebSocket', FakeWebSocket);
    resetStore();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    resetStore();
  });

  function sendBatch(socket: FakeWebSocket, changes: unknown[]): void {
    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({ type: 'batch', subId: 'main', changes }),
        }),
      );
    });
  }

  function openWithSnapshot(): FakeWebSocket {
    const socket = openFeedFor('main');
    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({ type: 'snapshot', subId: 'main', columns: makeColumns([]) }),
        }),
      );
    });
    return socket;
  }

  it('applies every change it carries', async () => {
    const socket = openWithSnapshot();

    sendBatch(socket, [
      { type: 'added', row: makeRow({ uid: 'a', name: 'pod-a' }) },
      { type: 'added', row: makeRow({ uid: 'b', name: 'pod-b' }) },
      { type: 'modified', row: makeRow({ uid: 'b', name: 'pod-b2' }) },
      { type: 'deleted', uid: 'a' },
    ]);
    await flushDeltas();

    const rows = useResourcesStore.getState().subs.get('main')?.rows;
    expect([...(rows?.keys() ?? [])]).toEqual(['b']);
    expect(rows?.get('b')?.name).toBe('pod-b2');
  });

  it('ignores a change it cannot read', async () => {
    const socket = openWithSnapshot();

    sendBatch(socket, [{ type: 'nonsense' }, { type: 'added', row: makeRow({ uid: 'c' }) }]);
    await flushDeltas();

    const rows = useResourcesStore.getState().subs.get('main')?.rows;
    expect([...(rows?.keys() ?? [])]).toEqual(['c']);
  });

  it('leaves a batch for a subscription nobody asked for alone', async () => {
    const socket = openWithSnapshot();

    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({
            type: 'batch',
            subId: 'other',
            changes: [{ type: 'added', row: makeRow({ uid: 'x' }) }],
          }),
        }),
      );
    });
    await flushDeltas();

    expect(useResourcesStore.getState().subs.get('other')).toBeUndefined();
  });
});

describe('the server saying which cluster it is on', () => {
  beforeEach(() => {
    FakeWebSocket.instances = [];
    vi.stubGlobal('WebSocket', FakeWebSocket);
    useContextsStore.getState().reset();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    useContextsStore.getState().reset();
  });

  function announce(socket: FakeWebSocket, context: string): void {
    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', { data: JSON.stringify({ type: 'context', context }) }),
      );
    });
  }

  it('adopts a cluster this window did not switch to', async () => {
    const list = {
      current: { kubeconfig: '', name: 'elsewhere' },
      kubeconfigs: [],
      protection: 'open',
    };
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(list) }),
    );
    renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
    });

    announce(socket, 'elsewhere');

    await waitFor(() => {
      expect(globalThis.fetch).toHaveBeenCalledWith('/api/contexts', expect.anything());
    });
    await waitFor(() => {
      expect(useContextsStore.getState().list.current.name).toBe('elsewhere');
    });
  });

  it('says nothing when the cluster is the one it already knows', () => {
    useContextsStore.getState().setList({
      current: { kubeconfig: '', name: 'p-mk1' },
      kubeconfigs: [],
      protection: 'open',
    });
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
    });

    announce(socket, 'p-mk1');

    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('ignores an announcement with no cluster in it', () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
    });

    announce(socket, '');

    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe('the server saying whether the cluster answers', () => {
  beforeEach(() => {
    FakeWebSocket.instances = [];
    vi.stubGlobal('WebSocket', FakeWebSocket);
    useClusterHealthStore.getState().reset();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    useClusterHealthStore.getState().reset();
  });

  function say(socket: FakeWebSocket, reachable: boolean, reason?: string): void {
    act(() => {
      socket.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({ type: 'cluster', reachable, reason }),
        }),
      );
    });
  }

  it('records a cluster that stopped answering, with the reason', () => {
    renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
    });

    say(socket, false, 'dial tcp 10.0.0.1:6443: connect: connection refused');

    expect(useClusterHealthStore.getState().byCluster['']).toMatchObject({
      reachable: false,
      reason: 'dial tcp 10.0.0.1:6443: connect: connection refused',
    });
  });

  it('records a cluster that came back', () => {
    renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
    });
    say(socket, false, 'connection refused');

    say(socket, true);

    expect(useClusterHealthStore.getState().byCluster['']).toMatchObject({
      reachable: true,
      reason: '',
    });
  });

  it('takes the frame even though it names no subscription', () => {
    renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
    });

    say(socket, false, 'gone');

    expect(useClusterHealthStore.getState().byCluster['']?.reachable).toBe(false);
  });
});

describe('which cluster a subscription is for', () => {
  beforeEach(() => {
    FakeWebSocket.instances = [];
    vi.stubGlobal('WebSocket', FakeWebSocket);
    resetStore();
  });

  afterEach(() => {
    setActiveCluster('');
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
    vi.useRealTimers();
    resetStore();
  });

  it('says nothing about the cluster while only one is open', () => {
    const socket = openFeedFor('main');

    expect(sentMessages(socket)[0]).not.toHaveProperty('cluster');
  });

  it('names the cluster the window is looking at', () => {
    setActiveCluster(mk2);

    const socket = openFeedFor('main');

    expect(sentMessages(socket)[0]).toMatchObject({ subId: 'main', cluster: mk2 });
  });

  it('names the cluster on a log stream too', () => {
    setActiveCluster(mk2);
    const { result } = renderHook(() => useResourceFeed());
    const socket = FakeWebSocket.instances[0];
    act(() => {
      openSocket(socket);
    });

    act(() => {
      result.current.subscribeLogs('l1', logRequest);
    });

    expect(sentMessages(socket)[0]).toMatchObject({ subId: 'l1', cluster: mk2 });
  });

  it('replays a subscription on the cluster it was made for, not the one now in front', () => {
    vi.useFakeTimers();
    setActiveCluster(mk2);
    const { result } = renderHook(() => useResourceFeed());
    const first = FakeWebSocket.instances[0];
    act(() => {
      openSocket(first);
    });
    act(() => {
      result.current.subscribe('main', descriptor, 'prod', []);
    });
    setActiveCluster(mk1);

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

    expect(sentMessages(second)[0]).toMatchObject({ subId: 'main', cluster: mk2 });
  });

  it('replays a log stream on the cluster it was opened for', () => {
    vi.useFakeTimers();
    setActiveCluster(mk2);
    const { result } = renderHook(() => useResourceFeed());
    const first = FakeWebSocket.instances[0];
    act(() => {
      openSocket(first);
    });
    act(() => {
      result.current.subscribeLogs('l1', logRequest);
    });
    setActiveCluster(mk1);

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

    expect(sentMessages(second)[0]).toMatchObject({ subId: 'l1', cluster: mk2 });
  });
});

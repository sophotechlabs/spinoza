import { useCallback, useEffect, useRef, useState } from 'react';
import type { ClientMsg, LogRequest, ResourceDescriptor, ServerMsg } from './types';
import { parseColumn, parseRow } from './parse';
import { asBoolean, asList, asRecord, asString, listOf } from './wire';
import { useResourcesStore } from '../store/resources';
import { useLogsStore } from '../store/logs';
import { wsURL } from './wsBase';

export type ConnectionStatus = 'connecting' | 'connected' | 'disconnected';

interface Subscription {
  descriptor: ResourceDescriptor;
  namespace: string;
}

export interface ResourceFeed {
  status: ConnectionStatus;
  subscribe: (subId: string, descriptor: ResourceDescriptor, namespace: string) => void;
  unsubscribe: (subId: string) => void;
  subscribeLogs: (subId: string, request: LogRequest) => void;
  unsubscribeLogs: (subId: string) => void;
  reconnect: () => void;
}

const BASE_BACKOFF_MS = 500;
const MAX_BACKOFF_MS = 5000;
const OPEN_STATE = 1;
export const DELTA_FLUSH_MS = 100;

function subscribeMsg(subId: string, sub: Subscription): ClientMsg {
  return {
    type: 'subscribe',
    subId,
    group: sub.descriptor.group,
    version: sub.descriptor.version,
    resource: sub.descriptor.resource,
    namespace: sub.namespace,
  };
}

function logsMsg(subId: string, request: LogRequest): ClientMsg {
  return {
    type: 'logs-subscribe',
    subId,
    namespace: request.namespace,
    name: request.name,
    container: request.container,
    tailLines: request.tailLines,
    follow: request.follow,
  };
}

function send(socket: WebSocket, msg: ClientMsg): void {
  socket.send(JSON.stringify(msg));
}

function canSend(socket: WebSocket | null): socket is WebSocket {
  if (socket === null) {
    return false;
  }
  return socket.readyState === OPEN_STATE;
}

function serverMsg(raw: unknown): ServerMsg | null {
  const item = asRecord(raw);
  const subId = asString(item.subId);
  switch (item.type) {
    case 'snapshot':
      return {
        type: 'snapshot',
        subId,
        columns: listOf(item.columns, parseColumn),
        namespaced: asBoolean(item.namespaced),
        rows: listOf(item.rows, parseRow),
      };
    case 'added':
      return { type: 'added', subId, row: parseRow(asRecord(item.row)) };
    case 'modified':
      return { type: 'modified', subId, row: parseRow(asRecord(item.row)) };
    case 'deleted':
      return { type: 'deleted', subId, uid: asString(item.uid) };
    case 'log':
      return { type: 'log', subId, lines: asList(item.lines).map(asString) };
    case 'log-end':
      return { type: 'log-end', subId };
    case 'error':
      return { type: 'error', subId, message: asString(item.message) };
    default:
      return null;
  }
}

export function useResourceFeed(): ResourceFeed {
  const [status, setStatus] = useState<ConnectionStatus>('connecting');
  const socketRef = useRef<WebSocket | null>(null);
  const subsRef = useRef<Map<string, Subscription>>(new Map());
  const logSubsRef = useRef<Map<string, LogRequest>>(new Map());
  const reconnectRef = useRef<(() => void) | null>(null);

  useEffect(() => {
    let disposed = false;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let attempt = 0;
    const store = useResourcesStore.getState();
    const logs = useLogsStore.getState();
    const pending = new Map<string, ServerMsg[]>();
    const pendingLines = new Map<string, string[]>();
    let flushTimer: ReturnType<typeof setTimeout> | null = null;

    function flush() {
      if (flushTimer !== null) {
        clearTimeout(flushTimer);
        flushTimer = null;
      }
      for (const [subId, msgs] of pending) {
        store.applyDeltas(subId, msgs);
      }
      pending.clear();
      for (const [subId, lines] of pendingLines) {
        logs.appendLines(subId, lines);
      }
      pendingLines.clear();
    }

    function schedule() {
      flushTimer ??= setTimeout(flush, DELTA_FLUSH_MS);
    }

    function queue(msg: ServerMsg) {
      const waiting = pending.get(msg.subId);
      if (waiting === undefined) {
        pending.set(msg.subId, [msg]);
      } else {
        waiting.push(msg);
      }
      schedule();
    }

    function queueLines(subId: string, lines: string[]) {
      const waiting = pendingLines.get(subId);
      if (waiting === undefined) {
        pendingLines.set(subId, [...lines]);
      } else {
        waiting.push(...lines);
      }
      schedule();
    }

    function dropPending(subId: string) {
      pending.delete(subId);
    }

    function clearFlush() {
      if (flushTimer !== null) {
        clearTimeout(flushTimer);
        flushTimer = null;
      }
      pending.clear();
      pendingLines.clear();
    }

    function clearTimer() {
      if (reconnectTimer !== null) {
        clearTimeout(reconnectTimer);
        reconnectTimer = null;
      }
    }

    function scheduleReconnect() {
      const delay = Math.min(MAX_BACKOFF_MS, BASE_BACKOFF_MS * 2 ** attempt);
      attempt += 1;
      reconnectTimer = setTimeout(connect, delay);
    }

    function resubscribeAll(socket: WebSocket) {
      for (const [subId, sub] of subsRef.current) {
        send(socket, subscribeMsg(subId, sub));
      }
      for (const [subId, request] of logSubsRef.current) {
        logs.startStream(subId);
        send(socket, logsMsg(subId, request));
      }
    }

    function knownSub(msg: ServerMsg): boolean {
      if (msg.type === 'log' || msg.type === 'log-end') {
        return logSubsRef.current.has(msg.subId);
      }
      if (msg.type === 'error') {
        if (subsRef.current.has(msg.subId)) {
          return true;
        }
        return logSubsRef.current.has(msg.subId);
      }
      return subsRef.current.has(msg.subId);
    }

    function handleMessage(event: MessageEvent) {
      if (disposed) {
        return;
      }
      let raw: unknown = null;
      try {
        raw = JSON.parse(event.data as string);
      } catch {
        return;
      }
      const msg = serverMsg(raw);
      if (msg === null) {
        return;
      }
      if (!knownSub(msg)) {
        return;
      }
      switch (msg.type) {
        case 'snapshot':
          dropPending(msg.subId);
          store.applySnapshot(msg.subId, msg.columns, msg.namespaced, msg.rows);
          break;
        case 'added':
        case 'modified':
        case 'deleted':
          queue(msg);
          break;
        case 'log':
          queueLines(msg.subId, msg.lines);
          break;
        case 'log-end':
          flush();
          logs.endStream(msg.subId);
          break;
        case 'error':
          flush();
          if (subsRef.current.has(msg.subId)) {
            store.failSub(msg.subId, msg.message);
          }
          if (logSubsRef.current.has(msg.subId)) {
            logs.failStream(msg.subId, msg.message);
          }
          break;
      }
    }

    function detach(socket: WebSocket) {
      socket.onopen = null;
      socket.onmessage = null;
      socket.onerror = null;
      socket.onclose = null;
    }

    function connect() {
      if (disposed) {
        return;
      }
      setStatus('connecting');
      const ws = new WebSocket(wsURL('/ws'));
      socketRef.current = ws;

      ws.onopen = () => {
        if (disposed) {
          return;
        }
        attempt = 0;
        setStatus('connected');
        resubscribeAll(ws);
      };

      ws.onmessage = handleMessage;

      ws.onerror = () => {
        ws.close();
      };

      ws.onclose = () => {
        if (disposed) {
          return;
        }
        setStatus('disconnected');
        scheduleReconnect();
      };
    }

    function reconnect() {
      if (disposed) {
        return;
      }
      clearTimer();
      clearFlush();
      const socket = socketRef.current;
      if (socket !== null) {
        detach(socket);
        socket.close();
        socketRef.current = null;
      }
      attempt = 0;
      connect();
    }

    reconnectRef.current = reconnect;
    connect();

    return () => {
      disposed = true;
      clearTimer();
      clearFlush();
      const socket = socketRef.current;
      if (socket !== null) {
        detach(socket);
        socket.close();
        socketRef.current = null;
      }
    };
  }, []);

  const subscribe = useCallback(
    (subId: string, descriptor: ResourceDescriptor, namespace: string) => {
      const sub: Subscription = { descriptor, namespace };
      subsRef.current.set(subId, sub);
      const socket = socketRef.current;
      if (canSend(socket)) {
        send(socket, subscribeMsg(subId, sub));
      }
    },
    [],
  );

  const unsubscribe = useCallback((subId: string) => {
    subsRef.current.delete(subId);
    const socket = socketRef.current;
    if (canSend(socket)) {
      send(socket, { type: 'unsubscribe', subId });
    }
    useResourcesStore.getState().clearSub(subId);
  }, []);

  const subscribeLogs = useCallback((subId: string, request: LogRequest) => {
    logSubsRef.current.set(subId, request);
    useLogsStore.getState().startStream(subId);
    const socket = socketRef.current;
    if (canSend(socket)) {
      send(socket, logsMsg(subId, request));
    }
  }, []);

  const unsubscribeLogs = useCallback((subId: string) => {
    logSubsRef.current.delete(subId);
    const socket = socketRef.current;
    if (canSend(socket)) {
      send(socket, { type: 'logs-unsubscribe', subId });
    }
    useLogsStore.getState().clearStream(subId);
  }, []);

  const reconnect = useCallback(() => {
    const run = reconnectRef.current;
    if (run !== null) {
      run();
    }
  }, []);

  return { status, subscribe, unsubscribe, subscribeLogs, unsubscribeLogs, reconnect };
}

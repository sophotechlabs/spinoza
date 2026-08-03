import { useCallback, useEffect, useRef, useState } from 'react';
import type { ClientMsg, LogRequest, ResourceDescriptor, ServerMsg } from './types';
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

function listOr<T>(value: T[] | undefined): T[] {
  if (value === undefined) {
    return [];
  }
  return value;
}

function flagOr(value: boolean | undefined): boolean {
  if (value === undefined) {
    return false;
  }
  return value;
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
        return true;
      }
      return subsRef.current.has(msg.subId);
    }

    function handleMessage(event: MessageEvent) {
      if (disposed) {
        return;
      }
      let msg: ServerMsg;
      try {
        msg = JSON.parse(event.data as string) as ServerMsg;
      } catch {
        return;
      }
      if (!knownSub(msg)) {
        return;
      }
      switch (msg.type) {
        case 'snapshot':
          store.applySnapshot(
            msg.subId,
            listOr(msg.columns),
            flagOr(msg.namespaced),
            listOr(msg.rows),
          );
          break;
        case 'added':
        case 'modified':
        case 'deleted':
          store.applyDelta(msg.subId, msg);
          break;
        case 'log':
          logs.appendLines(msg.subId, msg.lines);
          break;
        case 'log-end':
          logs.endStream(msg.subId);
          break;
        case 'error':
          store.failSub(msg.subId, msg.message);
          logs.failStream(msg.subId, msg.message);
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

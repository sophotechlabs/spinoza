import { useEffect, useState } from 'react';
import type { ServerMsg } from './types';
import { usePodStore } from '../store/pods';

type ConnectionStatus = 'connecting' | 'connected' | 'disconnected';

export interface FeedStatus {
  status: ConnectionStatus;
}

const BASE_BACKOFF_MS = 500;
const MAX_BACKOFF_MS = 5000;

function wsBaseOverride(): string | null {
  const w = window as unknown as { __SPINOZA_WS_BASE__?: string };
  if (typeof w.__SPINOZA_WS_BASE__ === 'string') {
    return w.__SPINOZA_WS_BASE__;
  }
  return null;
}

function wsURL(): string {
  const override = wsBaseOverride();
  if (override !== null) {
    return `${override}/ws`;
  }
  let proto = 'ws';
  if (location.protocol === 'https:') {
    proto = 'wss';
  }
  return `${proto}://${location.host}/ws`;
}

export function usePodsFeed(): FeedStatus {
  const [status, setStatus] = useState<ConnectionStatus>('connecting');

  useEffect(() => {
    const applySnapshot = usePodStore.getState().applySnapshot;
    const applyDelta = usePodStore.getState().applyDelta;

    let socket: WebSocket | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let attempt = 0;
    let disposed = false;

    function scheduleReconnect() {
      const delay = Math.min(MAX_BACKOFF_MS, BASE_BACKOFF_MS * 2 ** attempt);
      attempt += 1;
      reconnectTimer = setTimeout(connect, delay);
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
      switch (msg.type) {
        case 'snapshot':
          applySnapshot(msg.items);
          break;
        case 'added':
        case 'modified':
        case 'deleted':
          applyDelta(msg);
          break;
        case 'error':
          console.error('pods feed error:', msg.message);
          setStatus('disconnected');
          break;
      }
    }

    function connect() {
      if (disposed) {
        return;
      }
      setStatus('connecting');
      const ws = new WebSocket(wsURL());
      socket = ws;

      ws.onopen = () => {
        if (disposed) {
          return;
        }
        attempt = 0;
        setStatus('connected');
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

    connect();

    return () => {
      disposed = true;
      if (reconnectTimer !== null) {
        clearTimeout(reconnectTimer);
      }
      if (socket !== null) {
        socket.onopen = null;
        socket.onmessage = null;
        socket.onerror = null;
        socket.onclose = null;
        socket.close();
      }
    };
  }, []);

  return { status };
}

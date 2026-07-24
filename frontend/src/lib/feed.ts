import { useEffect, useState } from 'react';
import type { ServerMsg } from './types';
import { usePodStore } from '../store/pods';

type ConnectionStatus = 'connecting' | 'connected' | 'disconnected';

export interface FeedStatus {
  status: ConnectionStatus;
}

const BASE_BACKOFF_MS = 500;
const MAX_BACKOFF_MS = 5000;

function wsURL(): string {
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
      if (disposed) {
        return;
      }
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
        msg = JSON.parse(event.data);
      } catch {
        return;
      }
      if (msg.type === 'snapshot') {
        applySnapshot(msg.items);
      } else if (msg.type === 'added' || msg.type === 'modified' || msg.type === 'deleted') {
        applyDelta(msg);
      } else if (msg.type === 'error') {
        console.error('pods feed error:', msg.message);
        setStatus('disconnected');
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

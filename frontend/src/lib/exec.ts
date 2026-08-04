import type { ExecSupport, ExecTarget } from './types';
import { failure } from './object';
import { request } from './http';
import { parseExecSupport } from './parse';
import { wsURL } from './wsBase';

export const CHANNEL_STDIN = 0x00;
export const CHANNEL_STDOUT = 0x01;
export const CHANNEL_STDERR = 0x02;
export const CHANNEL_ERROR = 0x03;
export const CHANNEL_RESIZE = 0x04;

const CONNECTING_STATE = 0;
const OPEN_STATE = 1;

export const CONNECT_FAILED = 'could not reach the exec endpoint';
export const CONNECTION_LOST = 'the exec connection dropped';

export interface ExecEnd {
  message: string;
  failed: boolean;
}

export interface ExecHandlers {
  onOutput: (text: string) => void;
  onEnd: (end: ExecEnd) => void;
}

export interface ExecSession {
  send: (data: string) => void;
  resize: (cols: number, rows: number) => void;
  close: () => void;
}

export function execQuery(target: ExecTarget): string {
  const params = new URLSearchParams({
    namespace: target.namespace,
    pod: target.pod,
  });
  if (target.container !== '') {
    params.set('container', target.container);
  }
  return params.toString();
}

export async function fetchExecSupport(target: ExecTarget): Promise<ExecSupport> {
  const response = await request(`/api/exec/support?${execQuery(target)}`);
  if (!response.ok) {
    throw await failure(response, `exec support failed with status ${response.status}`);
  }
  return parseExecSupport(await response.json());
}

export function frame(channel: number, payload: Uint8Array): Uint8Array {
  const out = new Uint8Array(payload.length + 1);
  out[0] = channel;
  out.set(payload, 1);
  return out;
}

export function textFrame(channel: number, text: string): Uint8Array {
  return frame(channel, new TextEncoder().encode(text));
}

export function openExec(target: ExecTarget, handlers: ExecHandlers): ExecSession {
  const socket = new WebSocket(wsURL(`/api/exec?${execQuery(target)}`));
  socket.binaryType = 'arraybuffer';
  let ended = false;
  let opened = false;

  function finish(message: string, failed: boolean) {
    if (ended) {
      return;
    }
    ended = true;
    handlers.onEnd({ message, failed });
  }

  const stdoutDecoder = new TextDecoder();
  const stderrDecoder = new TextDecoder();

  socket.onmessage = (event: MessageEvent) => {
    const data = new Uint8Array(event.data as ArrayBuffer);
    if (data.length === 0) {
      return;
    }
    const payload = data.subarray(1);
    if (data[0] === CHANNEL_STDOUT) {
      handlers.onOutput(stdoutDecoder.decode(payload, { stream: true }));
      return;
    }
    if (data[0] === CHANNEL_STDERR) {
      handlers.onOutput(stderrDecoder.decode(payload, { stream: true }));
      return;
    }
    if (data[0] === CHANNEL_ERROR) {
      const message = new TextDecoder().decode(payload);
      finish(message, message !== '');
    }
  };

  socket.onclose = () => {
    if (!opened) {
      finish(CONNECT_FAILED, true);
      return;
    }
    finish(CONNECTION_LOST, true);
  };

  socket.onerror = () => {
    socket.close();
  };

  let pending: Uint8Array[] = [];

  socket.onopen = () => {
    opened = true;
    const queued = pending;
    pending = [];
    for (const frame of queued) {
      socket.send(frame);
    }
  };

  function write(payload: Uint8Array) {
    if (socket.readyState === CONNECTING_STATE) {
      pending.push(payload);
      return;
    }
    if (socket.readyState !== OPEN_STATE) {
      return;
    }
    socket.send(payload);
  }

  return {
    send: (data: string) => {
      write(textFrame(CHANNEL_STDIN, data));
    },
    resize: (cols: number, rows: number) => {
      write(textFrame(CHANNEL_RESIZE, JSON.stringify({ cols, rows })));
    },
    close: () => {
      socket.onmessage = null;
      socket.onclose = null;
      socket.onerror = null;
      socket.close();
    },
  };
}

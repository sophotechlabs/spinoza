import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  CHANNEL_ERROR,
  CHANNEL_RESIZE,
  CHANNEL_STDERR,
  CHANNEL_STDIN,
  CHANNEL_STDOUT,
  execQuery,
  fetchExecSupport,
  frame,
  openExec,
  textFrame,
} from '../../src/lib/exec';
import type { ExecTarget } from '../../src/lib/types';

class FakeWebSocket {
  static instances: FakeWebSocket[] = [];
  url: string;
  binaryType = '';
  readyState = 1;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  send = vi.fn<(data: Uint8Array) => void>();
  close = vi.fn((): void => {
    this.readyState = 3;
  });

  constructor(url: string) {
    this.url = url;
    FakeWebSocket.instances.push(this);
  }
}

function latest(): FakeWebSocket {
  return FakeWebSocket.instances[FakeWebSocket.instances.length - 1];
}

function deliver(socket: FakeWebSocket, channel: number, text: string): void {
  const payload = textFrame(channel, text);
  socket.onmessage?.({ data: payload.buffer } as MessageEvent);
}

function target(overrides: Partial<ExecTarget> = {}): ExecTarget {
  return { namespace: 'monitoring', pod: 'loki-0', container: 'loki', ...overrides };
}

function handlers() {
  return { onOutput: vi.fn<(text: string) => void>(), onEnd: vi.fn<(message: string) => void>() };
}

function sentFrames(socket: FakeWebSocket): { channel: number; text: string }[] {
  return socket.send.mock.calls.map((call) => {
    const data = call[0];
    return { channel: data[0], text: new TextDecoder().decode(data.subarray(1)) };
  });
}

describe('exec frames', () => {
  it('prefixes the channel byte', () => {
    const encoded = frame(CHANNEL_STDIN, new Uint8Array([1, 2, 3]));
    expect(Array.from(encoded)).toEqual([CHANNEL_STDIN, 1, 2, 3]);
  });

  it('encodes text payloads', () => {
    const encoded = textFrame(CHANNEL_RESIZE, '{"cols":80}');
    expect(encoded[0]).toBe(CHANNEL_RESIZE);
    expect(new TextDecoder().decode(encoded.subarray(1))).toBe('{"cols":80}');
  });

  it('builds the query for a named container', () => {
    expect(execQuery(target())).toBe('namespace=monitoring&pod=loki-0&container=loki');
  });

  it('omits an empty container', () => {
    expect(execQuery(target({ container: '' }))).toBe('namespace=monitoring&pod=loki-0');
  });
});

describe('fetchExecSupport', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('returns the reported verdict', async () => {
    const support = { namespace: 'monitoring', pod: 'loki-0', container: 'loki', shell: 'absent' };
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve({ ok: true, json: () => Promise.resolve(support) })),
    );

    await expect(fetchExecSupport(target())).resolves.toEqual(support);
  });

  it('throws when the request fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve({ ok: false, status: 500, json: () => Promise.resolve({ message: 'no' }) }),
      ),
    );

    await expect(fetchExecSupport(target())).rejects.toThrow('no');
  });
});

describe('openExec', () => {
  beforeEach(() => {
    FakeWebSocket.instances = [];
    vi.stubGlobal('WebSocket', FakeWebSocket);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('dials the exec endpoint for the target', () => {
    openExec(target(), handlers());
    expect(latest().url).toBe(
      `ws://${location.host}/api/exec?namespace=monitoring&pod=loki-0&container=loki`,
    );
    expect(latest().binaryType).toBe('arraybuffer');
  });

  it('reports stdout and stderr as output', () => {
    const sink = handlers();
    openExec(target(), sink);

    deliver(latest(), CHANNEL_STDOUT, '/ # ');
    deliver(latest(), CHANNEL_STDERR, 'oops');

    expect(sink.onOutput).toHaveBeenNthCalledWith(1, '/ # ');
    expect(sink.onOutput).toHaveBeenNthCalledWith(2, 'oops');
  });

  it('ends once on an error frame', () => {
    const sink = handlers();
    openExec(target(), sink);

    deliver(latest(), CHANNEL_ERROR, 'no /bin/sh');
    latest().onclose?.({} as CloseEvent);

    expect(sink.onEnd).toHaveBeenCalledTimes(1);
    expect(sink.onEnd).toHaveBeenCalledWith('no /bin/sh');
  });

  it('ends on a plain close', () => {
    const sink = handlers();
    openExec(target(), sink);

    latest().onclose?.({} as CloseEvent);

    expect(sink.onEnd).toHaveBeenCalledWith('');
  });

  it('ignores empty and unknown frames', () => {
    const sink = handlers();
    openExec(target(), sink);

    latest().onmessage?.({ data: new Uint8Array([]).buffer } as MessageEvent);
    deliver(latest(), 0x09, 'junk');

    expect(sink.onOutput).not.toHaveBeenCalled();
    expect(sink.onEnd).not.toHaveBeenCalled();
  });

  it('sends keystrokes and resizes', () => {
    const session = openExec(target(), handlers());

    session.send('ls\n');
    session.resize(120, 40);

    expect(sentFrames(latest())).toEqual([
      { channel: CHANNEL_STDIN, text: 'ls\n' },
      { channel: CHANNEL_RESIZE, text: '{"cols":120,"rows":40}' },
    ]);
  });

  it('drops writes once the socket is gone', () => {
    const session = openExec(target(), handlers());
    latest().readyState = 3;

    session.send('ls\n');

    expect(latest().send).not.toHaveBeenCalled();
  });

  it('closes the socket on error and on request', () => {
    const session = openExec(target(), handlers());

    latest().onerror?.(new Event('error'));
    session.close();

    expect(latest().close).toHaveBeenCalledTimes(2);
  });
});

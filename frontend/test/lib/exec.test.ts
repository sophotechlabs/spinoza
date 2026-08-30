import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  CHANNEL_ERROR,
  fetchNodeShellSupport,
  openNodeShell,
  CONNECT_FAILED,
  CONNECTION_LOST,
  CHANNEL_RESIZE,
  CHANNEL_STDERR,
  CHANNEL_STDIN,
  CHANNEL_STDOUT,
  execQuery,
  fetchExecSupport,
  fetchLocalShellSupport,
  frame,
  openExec,
  openLocalShell,
  textFrame,
} from '../../src/lib/exec';
import type { ExecEnd } from '../../src/lib/exec';
import type { ExecTarget } from '../../src/lib/types';
import { capabilities } from '../helpers';

class FakeWebSocket {
  static instances: FakeWebSocket[] = [];
  url: string;
  binaryType = '';
  readyState = 1;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onopen: ((event: Event) => void) | null = null;
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
  return { onOutput: vi.fn<(text: string) => void>(), onEnd: vi.fn<(end: ExecEnd) => void>() };
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
    expect(sink.onEnd).toHaveBeenCalledWith({ message: 'no /bin/sh', failed: true });
  });

  it('ends cleanly on the empty error frame the server sends at exit', () => {
    const sink = handlers();
    openExec(target(), sink);
    latest().onopen?.(new Event('open'));

    deliver(latest(), CHANNEL_ERROR, '');

    expect(sink.onEnd).toHaveBeenCalledWith({ message: '', failed: false });
  });

  it('names a failed handshake instead of pretending the shell exited', () => {
    const sink = handlers();
    openExec(target(), sink);

    latest().onclose?.({} as CloseEvent);

    expect(sink.onEnd).toHaveBeenCalledWith({ message: CONNECT_FAILED, failed: true });
  });

  it('names a connection that dropped once the shell was live', () => {
    const sink = handlers();
    openExec(target(), sink);
    latest().onopen?.(new Event('open'));

    latest().onclose?.({} as CloseEvent);

    expect(sink.onEnd).toHaveBeenCalledWith({ message: CONNECTION_LOST, failed: true });
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

function deliverBytes(socket: FakeWebSocket, channel: number, bytes: number[]): void {
  const payload = new Uint8Array([channel, ...bytes]);
  socket.onmessage?.({ data: payload.buffer } as MessageEvent);
}

describe('openExec output decoding', () => {
  beforeEach(() => {
    FakeWebSocket.instances = [];
    vi.stubGlobal('WebSocket', FakeWebSocket);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('joins a multi-byte character split across two frames', () => {
    const sink = handlers();
    openExec(target(), sink);
    const socket = latest();

    const cyrillic = new TextEncoder().encode('привет');
    deliverBytes(socket, CHANNEL_STDOUT, [...cyrillic.subarray(0, 5)]);
    deliverBytes(socket, CHANNEL_STDOUT, [...cyrillic.subarray(5)]);

    expect(sink.onOutput.mock.calls.map((call) => call[0]).join('')).toBe('привет');
  });

  it('does not emit a replacement character for a split box-drawing glyph', () => {
    const sink = handlers();
    openExec(target(), sink);
    const socket = latest();

    const glyph = new TextEncoder().encode('└─┐');
    deliverBytes(socket, CHANNEL_STDOUT, [...glyph.subarray(0, 2)]);
    deliverBytes(socket, CHANNEL_STDOUT, [...glyph.subarray(2)]);

    const joined = sink.onOutput.mock.calls.map((call) => call[0]).join('');
    expect(joined).toBe('└─┐');
    expect(joined).not.toContain('\ufffd');
  });

  it('keeps stdout and stderr in separate decoders', () => {
    const sink = handlers();
    openExec(target(), sink);
    const socket = latest();

    const out = new TextEncoder().encode('привет');
    const err = new TextEncoder().encode('ошибка');
    deliverBytes(socket, CHANNEL_STDOUT, [...out.subarray(0, 5)]);
    deliverBytes(socket, CHANNEL_STDERR, [...err.subarray(0, 5)]);
    deliverBytes(socket, CHANNEL_STDOUT, [...out.subarray(5)]);
    deliverBytes(socket, CHANNEL_STDERR, [...err.subarray(5)]);

    const emitted = sink.onOutput.mock.calls.map((call) => call[0]);
    expect(emitted.join('')).not.toContain('\ufffd');
    expect(emitted[0] + emitted[2]).toBe('привет');
    expect(emitted[1] + emitted[3]).toBe('ошибка');
  });
});

class ConnectingSocket extends FakeWebSocket {
  constructor(url: string) {
    super(url);
    this.readyState = 0;
  }
}

describe('openExec frames sent before the socket opens', () => {
  beforeEach(() => {
    FakeWebSocket.instances = [];
    vi.stubGlobal('WebSocket', ConnectingSocket);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('holds the initial terminal size until the handshake finishes', () => {
    const session = openExec(target(), handlers());
    const socket = latest();
    session.resize(120, 40);

    expect(socket.send).not.toHaveBeenCalled();

    socket.readyState = 1;
    socket.onopen?.(new Event('open'));

    const sent = sentFrames(socket);
    expect(sent).toHaveLength(1);
    expect(sent[0].channel).toBe(CHANNEL_RESIZE);
    expect(JSON.parse(sent[0].text)).toEqual({ cols: 120, rows: 40 });
  });

  it('replays queued keystrokes in order once open', () => {
    const session = openExec(target(), handlers());
    const socket = latest();
    session.send('a');
    session.send('b');

    socket.readyState = 1;
    socket.onopen?.(new Event('open'));

    expect(sentFrames(socket).map((entry) => entry.text)).toEqual(['a', 'b']);
  });

  it('stops calling back once the caller has closed the session', () => {
    const sink = handlers();
    const session = openExec(target(), sink);
    const socket = latest();

    session.close();
    socket.onmessage?.({ data: textFrame(CHANNEL_STDOUT, 'late').buffer } as MessageEvent);
    socket.onclose?.(new CloseEvent('close'));

    expect(sink.onOutput).not.toHaveBeenCalled();
    expect(sink.onEnd).not.toHaveBeenCalled();
  });

  it('drops frames written after the socket closed', () => {
    const session = openExec(target(), handlers());
    const socket = latest();
    socket.readyState = 3;

    session.send('late');

    expect(socket.send).not.toHaveBeenCalled();
  });
});

describe('the shell on this machine', () => {
  beforeEach(() => {
    FakeWebSocket.instances = [];
    vi.stubGlobal('WebSocket', FakeWebSocket);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('opens its own endpoint', () => {
    openLocalShell(handlers());

    expect(latest().url).toContain('/api/shell');
    expect(latest().url).not.toContain('/api/exec');
  });

  it('opens a node shell against the node it was given', () => {
    openNodeShell('p-mk1', handlers());

    expect(latest().url).toContain('/api/nodeshell?node=p-mk1');
    expect(latest().url).not.toContain('/api/exec');
  });

  it('carries what is typed like any other session', () => {
    const session = openLocalShell(handlers());
    const socket = latest();
    socket.readyState = 1;

    session.send('ls\n');

    expect(socket.send).toHaveBeenCalledWith(textFrame(CHANNEL_STDIN, 'ls\n'));
  });

  it('reports whether the desktop app offers one', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve(capabilities({ localShell: { available: true, reason: undefined } })),
        }),
      ),
    );

    await expect(fetchLocalShellSupport()).resolves.toEqual({
      available: true,
      reason: undefined,
    });
  });

  it('passes on the reason a browser tab cannot have one', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve({
          ok: true,
          json: () => Promise.resolve(capabilities()),
        }),
      ),
    );

    await expect(fetchLocalShellSupport()).resolves.toEqual({
      available: false,
      reason: 'desktop only',
    });
  });

  it('throws when the lookup fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve({ ok: false, status: 500, json: () => Promise.resolve({ message: 'no' }) }),
      ),
    );

    await expect(fetchLocalShellSupport()).rejects.toThrow('capabilities request failed');
  });
});

describe('the node shell endpoints', () => {
  it('names the node when asking whether one can be opened', async () => {
    const mock = vi.fn().mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          node: 'p-mk1',
          enabled: true,
          allowed: true,
          image: 'busybox:1.37',
          namespace: 'kube-system',
        }),
    });
    vi.stubGlobal('fetch', mock);

    const support = await fetchNodeShellSupport('p-mk1');

    expect(mock.mock.calls[0][0]).toBe('/api/nodeshell/support?node=p-mk1');
    expect(support).toMatchObject({ node: 'p-mk1', enabled: true, allowed: true });
  });

  it('surfaces a refusal', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 500,
        json: () => Promise.resolve({ message: 'the apiserver said no' }),
      }),
    );

    await expect(fetchNodeShellSupport('p-mk1')).rejects.toThrow('the apiserver said no');
  });
});

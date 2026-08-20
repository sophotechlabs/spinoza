import { beforeEach, describe, expect, it } from 'vitest';
import { renderHook } from '@testing-library/react';
import {
  MAX_LOG_LINES,
  useLogEnded,
  useLogLines,
  useLogPods,
  useLogResumed,
  useLogSources,
  useLogsStore,
} from '../../src/store/logs';

function reset(): void {
  useLogsStore.setState({ streams: new Map() });
}

describe('logs store', () => {
  beforeEach(reset);

  it('starts an empty stream', () => {
    useLogsStore.getState().startStream('logs');

    expect(useLogsStore.getState().streams.get('logs')).toEqual({
      lines: [],
      sources: [],
      dropped: 0,
      revision: 0,
      ended: false,
      resumed: false,
      attached: 0,
      matched: 0,
    });
  });

  it('appends lines in order', () => {
    useLogsStore.getState().startStream('logs');
    useLogsStore.getState().appendLines('logs', ['a', 'b'], '');
    useLogsStore.getState().appendLines('logs', ['c'], '');

    expect(useLogsStore.getState().streams.get('logs')?.lines).toEqual(['a', 'b', 'c']);
  });

  it('ignores appends for an unknown stream', () => {
    useLogsStore.getState().appendLines('missing', ['a'], '');

    expect(useLogsStore.getState().streams.size).toBe(0);
  });

  it('trims to the line cap', () => {
    useLogsStore.getState().startStream('logs');
    const lines = Array.from({ length: MAX_LOG_LINES + 10 }, (_, i) => `line-${i}`);
    useLogsStore.getState().appendLines('logs', lines, '');

    const stored = useLogsStore.getState().streams.get('logs')?.lines ?? [];
    expect(stored).toHaveLength(MAX_LOG_LINES);
    expect(stored[stored.length - 1]).toBe(`line-${MAX_LOG_LINES + 9}`);
  });

  it('marks a stream as ended', () => {
    useLogsStore.getState().startStream('logs');
    useLogsStore.getState().endStream('logs');

    expect(useLogsStore.getState().streams.get('logs')?.ended).toBe(true);
  });

  it('ignores ending an unknown stream', () => {
    useLogsStore.getState().endStream('missing');

    expect(useLogsStore.getState().streams.size).toBe(0);
  });

  it('records why a stream failed', () => {
    useLogsStore.getState().startStream('logs');
    useLogsStore.getState().failStream('logs', 'pods/log is forbidden');

    expect(useLogsStore.getState().streams.get('logs')).toMatchObject({
      ended: true,
      error: 'pods/log is forbidden',
    });
  });

  it('ignores failing an unknown stream', () => {
    useLogsStore.getState().failStream('missing', 'too late');

    expect(useLogsStore.getState().streams.size).toBe(0);
  });

  it('clears a stream', () => {
    useLogsStore.getState().startStream('logs');
    useLogsStore.getState().clearStream('logs');

    expect(useLogsStore.getState().streams.has('logs')).toBe(false);
  });

  it('ignores clearing an unknown stream', () => {
    useLogsStore.getState().startStream('logs');
    useLogsStore.getState().clearStream('missing');

    expect(useLogsStore.getState().streams.size).toBe(1);
  });

  it('exposes lines and the ended flag through hooks', () => {
    const lines = renderHook(() => useLogLines('logs'));
    const ended = renderHook(() => useLogEnded('logs'));

    expect(lines.result.current).toEqual([]);
    expect(ended.result.current).toBe(false);

    useLogsStore.getState().startStream('logs');
    useLogsStore.getState().appendLines('logs', ['a'], '');
    useLogsStore.getState().endStream('logs');
    lines.rerender();
    ended.rerender();

    expect(lines.result.current).toEqual(['a']);
    expect(ended.result.current).toBe(true);
  });
});

describe('a stream merged from several pods', () => {
  beforeEach(reset);

  it('remembers which pod wrote each line', () => {
    useLogsStore.getState().startStream('logs');
    useLogsStore.getState().appendLines('logs', ['one', 'two'], 'web-0');
    useLogsStore.getState().appendLines('logs', ['three'], 'web-1');

    const stream = useLogsStore.getState().streams.get('logs');
    expect(stream?.lines).toEqual(['one', 'two', 'three']);
    expect(stream?.sources).toEqual(['web-0', 'web-0', 'web-1']);
  });

  it('drops the oldest sources with the lines they belong to', () => {
    useLogsStore.getState().startStream('logs');
    const lines = Array.from({ length: MAX_LOG_LINES }, (_, i) => `old-${i}`);
    useLogsStore.getState().appendLines('logs', lines, 'web-0');
    useLogsStore.getState().appendLines('logs', ['newest'], 'web-1');

    const stream = useLogsStore.getState().streams.get('logs');
    expect(stream?.sources).toHaveLength(MAX_LOG_LINES);
    expect(stream?.lines[MAX_LOG_LINES - 1]).toBe('newest');
    expect(stream?.sources[MAX_LOG_LINES - 1]).toBe('web-1');
  });

  it('forgets the sources when the buffer is cleared', () => {
    useLogsStore.getState().startStream('logs');
    useLogsStore.getState().appendLines('logs', ['one'], 'web-0');

    useLogsStore.getState().clearLines('logs');

    expect(useLogsStore.getState().streams.get('logs')?.sources).toEqual([]);
  });

  it('records how many pods are being read of how many there are', () => {
    useLogsStore.getState().startStream('logs');

    useLogsStore.getState().openedStream('logs', 3, 5);

    expect(useLogsStore.getState().streams.get('logs')).toMatchObject({
      attached: 3,
      matched: 5,
    });
  });

  it('takes a later count without disturbing the lines already read', () => {
    useLogsStore.getState().startStream('logs');
    useLogsStore.getState().openedStream('logs', 1, 1);
    useLogsStore.getState().appendLines('logs', ['one'], 'web-0');

    useLogsStore.getState().openedStream('logs', 2, 2);

    const stream = useLogsStore.getState().streams.get('logs');
    expect(stream?.attached).toBe(2);
    expect(stream?.lines).toEqual(['one']);
  });

  it('ignores a count for an unknown stream', () => {
    useLogsStore.getState().openedStream('missing', 2, 2);

    expect(useLogsStore.getState().streams.size).toBe(0);
  });

  it('exposes the sources and the pod count through hooks', () => {
    const sources = renderHook(() => useLogSources('logs'));
    const pods = renderHook(() => useLogPods('logs'));

    expect(sources.result.current).toEqual([]);
    expect(pods.result.current).toEqual({ attached: 0, matched: 0 });

    useLogsStore.getState().startStream('logs');
    useLogsStore.getState().appendLines('logs', ['one'], 'web-0');
    useLogsStore.getState().openedStream('logs', 2, 4);
    sources.rerender();
    pods.rerender();

    expect(sources.result.current).toEqual(['web-0']);
    expect(pods.result.current).toEqual({ attached: 2, matched: 4 });
  });
});

describe('clearing a buffer without dropping the stream', () => {
  it('empties the lines and keeps the stream alive', () => {
    useLogsStore.getState().startStream('logs');
    useLogsStore.getState().appendLines('logs', ['one', 'two'], '');

    useLogsStore.getState().clearLines('logs');

    const stream = useLogsStore.getState().streams.get('logs');
    expect(stream?.lines).toEqual([]);
    expect(stream?.ended).toBe(false);
  });

  it('bumps the revision so the view redraws', () => {
    useLogsStore.getState().startStream('logs');
    useLogsStore.getState().appendLines('logs', ['one'], '');
    const before = useLogsStore.getState().streams.get('logs')?.revision ?? 0;

    useLogsStore.getState().clearLines('logs');

    expect(useLogsStore.getState().streams.get('logs')?.revision).toBe(before + 1);
  });

  it('ignores a stream that is not there', () => {
    const before = useLogsStore.getState().streams;

    useLogsStore.getState().clearLines('missing');

    expect(useLogsStore.getState().streams).toBe(before);
  });

  it('drops the reconnect notice with the lines it referred to', () => {
    useLogsStore.getState().startStream('logs');
    useLogsStore.getState().appendLines('logs', ['one'], '');
    useLogsStore.getState().resumeStream('logs');

    useLogsStore.getState().clearLines('logs');

    expect(useLogsStore.getState().streams.get('logs')?.resumed).toBe(false);
  });
});

describe('resuming a stream after a reconnect', () => {
  beforeEach(reset);

  it('keeps the buffer the user was reading', () => {
    useLogsStore.getState().startStream('logs');
    useLogsStore.getState().appendLines('logs', ['one', 'two'], '');

    useLogsStore.getState().resumeStream('logs');

    const stream = useLogsStore.getState().streams.get('logs');
    expect(stream?.lines).toEqual(['one', 'two']);
    expect(stream?.resumed).toBe(true);
  });

  it('appends the fresh tail after what was already there', () => {
    useLogsStore.getState().startStream('logs');
    useLogsStore.getState().appendLines('logs', ['one'], '');
    useLogsStore.getState().resumeStream('logs');

    useLogsStore.getState().appendLines('logs', ['two'], '');

    expect(useLogsStore.getState().streams.get('logs')?.lines).toEqual(['one', 'two']);
  });

  it('lifts an ended stream and its error back to live', () => {
    useLogsStore.getState().startStream('logs');
    useLogsStore.getState().appendLines('logs', ['one'], '');
    useLogsStore.getState().failStream('logs', 'connection reset');

    useLogsStore.getState().resumeStream('logs');

    const stream = useLogsStore.getState().streams.get('logs');
    expect(stream?.ended).toBe(false);
    expect(stream?.error).toBeUndefined();
  });

  it('says nothing about a resume when there was no output to keep', () => {
    useLogsStore.getState().startStream('logs');

    useLogsStore.getState().resumeStream('logs');

    expect(useLogsStore.getState().streams.get('logs')?.resumed).toBe(false);
  });

  it('starts a stream the reconnect knows about but the store does not', () => {
    useLogsStore.getState().resumeStream('logs');

    expect(useLogsStore.getState().streams.get('logs')).toEqual({
      lines: [],
      sources: [],
      dropped: 0,
      revision: 0,
      ended: false,
      resumed: false,
      attached: 0,
      matched: 0,
    });
  });

  it('redraws the view on resume', () => {
    useLogsStore.getState().startStream('logs');
    useLogsStore.getState().appendLines('logs', ['one'], '');
    const before = useLogsStore.getState().streams.get('logs')?.revision ?? 0;

    useLogsStore.getState().resumeStream('logs');

    expect(useLogsStore.getState().streams.get('logs')?.revision).toBe(before + 1);
  });

  it('exposes the resumed flag through a hook', () => {
    const resumed = renderHook(() => useLogResumed('logs'));
    expect(resumed.result.current).toBe(false);

    useLogsStore.getState().startStream('logs');
    useLogsStore.getState().appendLines('logs', ['one'], '');
    useLogsStore.getState().resumeStream('logs');
    resumed.rerender();

    expect(resumed.result.current).toBe(true);
  });
});

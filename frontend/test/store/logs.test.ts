import { beforeEach, describe, expect, it } from 'vitest';
import { renderHook } from '@testing-library/react';
import { MAX_LOG_LINES, useLogEnded, useLogLines, useLogsStore } from '../../src/store/logs';

function reset(): void {
  useLogsStore.setState({ streams: new Map() });
}

describe('logs store', () => {
  beforeEach(reset);

  it('starts an empty stream', () => {
    useLogsStore.getState().startStream('logs');

    expect(useLogsStore.getState().streams.get('logs')).toEqual({
      lines: [],
      dropped: 0,
      revision: 0,
      ended: false,
    });
  });

  it('appends lines in order', () => {
    useLogsStore.getState().startStream('logs');
    useLogsStore.getState().appendLines('logs', ['a', 'b']);
    useLogsStore.getState().appendLines('logs', ['c']);

    expect(useLogsStore.getState().streams.get('logs')?.lines).toEqual(['a', 'b', 'c']);
  });

  it('ignores appends for an unknown stream', () => {
    useLogsStore.getState().appendLines('missing', ['a']);

    expect(useLogsStore.getState().streams.size).toBe(0);
  });

  it('trims to the line cap', () => {
    useLogsStore.getState().startStream('logs');
    const lines = Array.from({ length: MAX_LOG_LINES + 10 }, (_, i) => `line-${i}`);
    useLogsStore.getState().appendLines('logs', lines);

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
    useLogsStore.getState().appendLines('logs', ['a']);
    useLogsStore.getState().endStream('logs');
    lines.rerender();
    ended.rerender();

    expect(lines.result.current).toEqual(['a']);
    expect(ended.result.current).toBe(true);
  });
});

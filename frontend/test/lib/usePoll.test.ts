import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import { usePoll } from '../../src/lib/usePoll';
import { bumpClusterEpoch } from '../../src/store/cluster';

afterEach(() => {
  vi.useRealTimers();
});

function options(overrides: Partial<{ enabled: boolean; fallback: string }> = {}) {
  return { intervalMs: 1000, ...overrides };
}

describe('usePoll', () => {
  it('loads once on mount and again on every interval', async () => {
    vi.useFakeTimers();
    const fetcher = vi.fn().mockResolvedValue('one');
    renderHook(() => usePoll(fetcher, options()));

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(fetcher).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });
    expect(fetcher).toHaveBeenCalledTimes(3);
  });

  it('never stacks a second call on top of a slow one', async () => {
    vi.useFakeTimers();
    let release: (value: string) => void = () => undefined;
    const fetcher = vi.fn(
      () =>
        new Promise<string>((resolve) => {
          release = resolve;
        }),
    );
    renderHook(() => usePoll(fetcher, options()));

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000);
    });
    expect(fetcher).toHaveBeenCalledTimes(1);

    await act(async () => {
      release('done');
      await vi.advanceTimersByTimeAsync(1000);
    });
    expect(fetcher).toHaveBeenCalledTimes(2);
  });

  it('keeps the last good value and marks it stale when a poll fails', async () => {
    vi.useFakeTimers();
    let call = 0;
    const fetcher = vi.fn(() => {
      call += 1;
      if (call === 1) {
        return Promise.resolve('fresh');
      }
      return Promise.reject(new Error('backend is down'));
    });

    const { result } = renderHook(() => usePoll(fetcher, options()));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(result.current.data).toBe('fresh');
    expect(result.current.stale).toBe(false);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    expect(result.current.data).toBe('fresh');
    expect(result.current.error).toBe('backend is down');
    expect(result.current.stale).toBe(true);
  });

  it('clears the error once a later poll succeeds', async () => {
    vi.useFakeTimers();
    let call = 0;
    const fetcher = vi.fn(() => {
      call += 1;
      if (call === 1) {
        return Promise.reject(new Error('down'));
      }
      return Promise.resolve('back');
    });

    const { result } = renderHook(() => usePoll(fetcher, options()));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(result.current.error).toBe('down');
    expect(result.current.stale).toBe(false);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    expect(result.current.data).toBe('back');
    expect(result.current.error).toBeNull();
  });

  it('falls back to the given message when the rejection is not an Error', async () => {
    const fetcher = vi.fn().mockRejectedValue('nope');
    const { result } = renderHook(() =>
      usePoll(fetcher, options({ fallback: 'events request failed' })),
    );
    await waitFor(() => {
      expect(result.current.error).toBe('events request failed');
    });
  });

  it('uses a generic message when no fallback was given', async () => {
    const fetcher = vi.fn().mockRejectedValue('nope');
    const { result } = renderHook(() => usePoll(fetcher, options()));
    await waitFor(() => {
      expect(result.current.error).toBe('request failed');
    });
  });

  it('does not fetch at all while disabled', async () => {
    vi.useFakeTimers();
    const fetcher = vi.fn().mockResolvedValue('one');
    renderHook(() => usePoll(fetcher, options({ enabled: false })));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });
    expect(fetcher).not.toHaveBeenCalled();
  });

  it('drops what it holds when it is disabled mid-flight', async () => {
    const fetcher = vi.fn().mockResolvedValue('one');
    const { result, rerender } = renderHook(
      ({ enabled }: { enabled: boolean }) => usePoll(fetcher, options({ enabled })),
      { initialProps: { enabled: true } },
    );
    await waitFor(() => {
      expect(result.current.data).toBe('one');
    });

    rerender({ enabled: false });
    expect(result.current.data).toBeNull();
  });

  it('drops the previous cluster data and refetches when the epoch moves', async () => {
    const fetcher = vi.fn().mockResolvedValue('one');
    const { result } = renderHook(() => usePoll(fetcher, options()));
    await waitFor(() => {
      expect(result.current.data).toBe('one');
    });

    act(() => {
      bumpClusterEpoch();
    });
    expect(result.current.data).toBeNull();

    await waitFor(() => {
      expect(fetcher).toHaveBeenCalledTimes(2);
    });
  });

  it('refetches on demand when reload is called', async () => {
    const fetcher = vi.fn().mockResolvedValue('one');
    const { result } = renderHook(() => usePoll(fetcher, options()));
    await waitFor(() => {
      expect(fetcher).toHaveBeenCalledTimes(1);
    });

    act(() => {
      result.current.reload();
    });

    await waitFor(() => {
      expect(fetcher).toHaveBeenCalledTimes(2);
    });
  });

  it('ignores a response that lands after unmount', async () => {
    let release: (value: string) => void = () => undefined;
    const fetcher = vi.fn(
      () =>
        new Promise<string>((resolve) => {
          release = resolve;
        }),
    );
    const { unmount } = renderHook(() => usePoll(fetcher, options()));
    unmount();

    await act(async () => {
      release('late');
      await Promise.resolve();
    });
  });

  it('ignores a rejection that lands after unmount', async () => {
    let reject: (reason: unknown) => void = () => undefined;
    const fetcher = vi.fn(
      () =>
        new Promise<string>((_resolve, fail) => {
          reject = fail;
        }),
    );
    const { unmount } = renderHook(() => usePoll(fetcher, options()));
    unmount();

    await act(async () => {
      reject(new Error('late'));
      await Promise.resolve();
    });
  });
});

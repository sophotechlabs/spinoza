import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import { useNow } from '../../src/lib/useNow';

afterEach(() => {
  vi.useRealTimers();
});

describe('useNow', () => {
  it('advances so relative times stop being frozen', async () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useNow(1000));
    const first = result.current;

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });

    expect(result.current).toBeGreaterThan(first);
  });

  it('stops ticking once unmounted', async () => {
    vi.useFakeTimers();
    const { result, unmount } = renderHook(() => useNow(1000));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });
    const last = result.current;

    unmount();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });

    expect(result.current).toBe(last);
  });
});

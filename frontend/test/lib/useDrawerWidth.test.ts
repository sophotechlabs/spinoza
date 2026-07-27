import { describe, expect, it } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import {
  clampWidth,
  DEFAULT_DRAWER_WIDTH,
  MAX_DRAWER_WIDTH,
  MIN_DRAWER_WIDTH,
  useDrawerWidth,
} from '../../src/lib/useDrawerWidth';

function drag(clientX: number): void {
  window.dispatchEvent(new MouseEvent('mousemove', { clientX }));
}

describe('useDrawerWidth', () => {
  it('clamps to the allowed range', () => {
    expect(clampWidth(10)).toBe(MIN_DRAWER_WIDTH);
    expect(clampWidth(5000)).toBe(MAX_DRAWER_WIDTH);
    expect(clampWidth(600)).toBe(600);
  });

  it('starts at the default width', () => {
    const { result } = renderHook(() => useDrawerWidth());

    expect(result.current.width).toBe(DEFAULT_DRAWER_WIDTH);
  });

  it('ignores movement before a drag starts', () => {
    const { result } = renderHook(() => useDrawerWidth());

    act(() => {
      drag(100);
    });

    expect(result.current.width).toBe(DEFAULT_DRAWER_WIDTH);
  });

  it('widens as the handle is dragged left', () => {
    const { result } = renderHook(() => useDrawerWidth());

    act(() => {
      result.current.startResize(800);
    });
    act(() => {
      drag(700);
    });

    expect(result.current.width).toBe(DEFAULT_DRAWER_WIDTH + 100);
  });

  it('narrows as the handle is dragged right and clamps at the minimum', () => {
    const { result } = renderHook(() => useDrawerWidth());

    act(() => {
      result.current.startResize(800);
    });
    act(() => {
      drag(3000);
    });

    expect(result.current.width).toBe(MIN_DRAWER_WIDTH);
  });

  it('stops tracking after mouseup', () => {
    const { result } = renderHook(() => useDrawerWidth());

    act(() => {
      result.current.startResize(800);
    });
    act(() => {
      drag(700);
    });
    act(() => {
      window.dispatchEvent(new MouseEvent('mouseup'));
    });
    act(() => {
      drag(100);
    });

    expect(result.current.width).toBe(DEFAULT_DRAWER_WIDTH + 100);
  });
});

import { describe, expect, it } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import {
  clampDockHeight,
  clampSidebar,
  clampWidth,
  DEFAULT_DOCK_HEIGHT,
  DEFAULT_DRAWER_WIDTH,
  DEFAULT_SIDEBAR_WIDTH,
  MAX_DOCK_HEIGHT,
  MAX_DRAWER_WIDTH,
  MAX_SIDEBAR_WIDTH,
  MIN_DOCK_HEIGHT,
  MIN_DRAWER_WIDTH,
  MIN_SIDEBAR_WIDTH,
  useDockSize,
  useSidebarWidth,
} from '../../src/lib/usePanelWidth';

function drag(clientX: number): void {
  window.dispatchEvent(new MouseEvent('mousemove', { clientX }));
}

function dragY(clientY: number): void {
  window.dispatchEvent(new MouseEvent('mousemove', { clientY }));
}

describe('the right dock size', () => {
  it('clamps to the allowed range', () => {
    expect(clampWidth(10)).toBe(MIN_DRAWER_WIDTH);
    expect(clampWidth(5000)).toBe(MAX_DRAWER_WIDTH);
    expect(clampWidth(600)).toBe(600);
  });

  it('starts at the default width', () => {
    const { result } = renderHook(() => useDockSize('right'));

    expect(result.current.size).toBe(DEFAULT_DRAWER_WIDTH);
  });

  it('ignores movement before a drag starts', () => {
    const { result } = renderHook(() => useDockSize('right'));

    act(() => {
      drag(100);
    });

    expect(result.current.size).toBe(DEFAULT_DRAWER_WIDTH);
  });

  it('widens as the handle is dragged left', () => {
    const { result } = renderHook(() => useDockSize('right'));

    act(() => {
      result.current.startResize(800);
    });
    act(() => {
      drag(700);
    });

    expect(result.current.size).toBe(DEFAULT_DRAWER_WIDTH + 100);
  });

  it('narrows as the handle is dragged right and clamps at the minimum', () => {
    const { result } = renderHook(() => useDockSize('right'));

    act(() => {
      result.current.startResize(800);
    });
    act(() => {
      drag(3000);
    });

    expect(result.current.size).toBe(MIN_DRAWER_WIDTH);
  });

  it('stops tracking after mouseup', () => {
    const { result } = renderHook(() => useDockSize('right'));

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

    expect(result.current.size).toBe(DEFAULT_DRAWER_WIDTH + 100);
  });
});

describe('useSidebarWidth', () => {
  it('clamps to the allowed range', () => {
    expect(clampSidebar(10)).toBe(MIN_SIDEBAR_WIDTH);
    expect(clampSidebar(5000)).toBe(MAX_SIDEBAR_WIDTH);
    expect(clampSidebar(300)).toBe(300);
  });

  it('starts at the default width', () => {
    const { result } = renderHook(() => useSidebarWidth());

    expect(result.current.size).toBe(DEFAULT_SIDEBAR_WIDTH);
  });

  it('widens as the handle is dragged right', () => {
    const { result } = renderHook(() => useSidebarWidth());

    act(() => {
      result.current.startResize(224);
    });
    act(() => {
      drag(324);
    });

    expect(result.current.size).toBe(DEFAULT_SIDEBAR_WIDTH + 100);
  });

  it('narrows as the handle is dragged left and clamps at the minimum', () => {
    const { result } = renderHook(() => useSidebarWidth());

    act(() => {
      result.current.startResize(224);
    });
    act(() => {
      drag(0);
    });

    expect(result.current.size).toBe(MIN_SIDEBAR_WIDTH);
  });

  it('nudges within the range', () => {
    const { result } = renderHook(() => useSidebarWidth());

    act(() => {
      result.current.nudge(32);
    });

    expect(result.current.size).toBe(DEFAULT_SIDEBAR_WIDTH + 32);
  });
});

describe('the left dock size', () => {
  it('widens as its handle is dragged right', () => {
    const { result } = renderHook(() => useDockSize('left'));

    act(() => {
      result.current.startResize(560);
    });
    act(() => {
      drag(660);
    });

    expect(result.current.size).toBe(DEFAULT_DRAWER_WIDTH + 100);
  });
});

describe('the bottom dock size', () => {
  it('clamps to the allowed range', () => {
    expect(clampDockHeight(10)).toBe(MIN_DOCK_HEIGHT);
    expect(clampDockHeight(5000)).toBe(MAX_DOCK_HEIGHT);
    expect(clampDockHeight(300)).toBe(300);
  });

  it('starts at the default height', () => {
    const { result } = renderHook(() => useDockSize('bottom'));

    expect(result.current.size).toBe(DEFAULT_DOCK_HEIGHT);
  });

  it('grows as the handle is dragged up', () => {
    const { result } = renderHook(() => useDockSize('bottom'));

    act(() => {
      result.current.startResize(600);
    });
    act(() => {
      dragY(500);
    });

    expect(result.current.size).toBe(DEFAULT_DOCK_HEIGHT + 100);
  });

  it('shrinks as the handle is dragged down and clamps at the minimum', () => {
    const { result } = renderHook(() => useDockSize('bottom'));

    act(() => {
      result.current.startResize(600);
    });
    act(() => {
      dragY(2000);
    });

    expect(result.current.size).toBe(MIN_DOCK_HEIGHT);
  });
});

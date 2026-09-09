import { describe, expect, it } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import { CANVAS_FLOOR, tooNarrow, useNarrowWindow } from '../../src/lib/viewport';

function resizeTo(width: number): void {
  act(() => {
    window.innerWidth = width;
    window.dispatchEvent(new Event('resize'));
  });
}

describe('the width the canvas needs', () => {
  it('calls anything under the floor narrow', () => {
    expect(tooNarrow(CANVAS_FLOOR)).toBe(false);
    expect(tooNarrow(CANVAS_FLOOR - 1)).toBe(true);
  });

  it('follows the window as it is resized', () => {
    const { result } = renderHook(() => useNarrowWindow());

    expect(result.current).toBe(false);

    resizeTo(1000);
    expect(result.current).toBe(true);

    resizeTo(1600);
    expect(result.current).toBe(false);
  });
});

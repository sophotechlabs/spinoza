import { useCallback, useEffect, useRef, useState } from 'react';

export const MIN_DRAWER_WIDTH = 320;
export const MAX_DRAWER_WIDTH = 1200;
export const DEFAULT_DRAWER_WIDTH = 560;

interface DragStart {
  x: number;
  width: number;
}

export function clampWidth(width: number): number {
  if (width < MIN_DRAWER_WIDTH) {
    return MIN_DRAWER_WIDTH;
  }
  if (width > MAX_DRAWER_WIDTH) {
    return MAX_DRAWER_WIDTH;
  }
  return width;
}

export const DRAWER_NUDGE_STEP = 32;

export function useDrawerWidth(): {
  width: number;
  startResize: (clientX: number) => void;
  nudge: (delta: number) => void;
} {
  const [width, setWidth] = useState(DEFAULT_DRAWER_WIDTH);
  const startRef = useRef<DragStart | null>(null);

  useEffect(() => {
    function handleMove(event: MouseEvent) {
      const start = startRef.current;
      if (start === null) {
        return;
      }
      setWidth(clampWidth(start.width + (start.x - event.clientX)));
    }

    function handleUp() {
      startRef.current = null;
    }

    window.addEventListener('mousemove', handleMove);
    window.addEventListener('mouseup', handleUp);
    return () => {
      window.removeEventListener('mousemove', handleMove);
      window.removeEventListener('mouseup', handleUp);
    };
  }, []);

  const startResize = useCallback(
    (clientX: number) => {
      startRef.current = { x: clientX, width };
    },
    [width],
  );

  const nudge = useCallback((delta: number) => {
    setWidth((current) => clampWidth(current + delta));
  }, []);

  return { width, startResize, nudge };
}

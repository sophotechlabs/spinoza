import { useCallback, useEffect, useRef, useState } from 'react';

export const MIN_DRAWER_WIDTH = 320;
export const MAX_DRAWER_WIDTH = 1200;
export const DEFAULT_DRAWER_WIDTH = 560;

export const MIN_SIDEBAR_WIDTH = 160;
export const MAX_SIDEBAR_WIDTH = 560;
export const DEFAULT_SIDEBAR_WIDTH = 224;

export const NUDGE_STEP = 32;

interface Limits {
  min: number;
  max: number;
  initial: number;
  sign: number;
}

const DRAWER: Limits = {
  min: MIN_DRAWER_WIDTH,
  max: MAX_DRAWER_WIDTH,
  initial: DEFAULT_DRAWER_WIDTH,
  sign: -1,
};

const SIDEBAR: Limits = {
  min: MIN_SIDEBAR_WIDTH,
  max: MAX_SIDEBAR_WIDTH,
  initial: DEFAULT_SIDEBAR_WIDTH,
  sign: 1,
};

interface DragStart {
  x: number;
  width: number;
}

export interface PanelWidth {
  width: number;
  startResize: (clientX: number) => void;
  nudge: (delta: number) => void;
}

function clamp(limits: Limits, width: number): number {
  if (width < limits.min) {
    return limits.min;
  }
  if (width > limits.max) {
    return limits.max;
  }
  return width;
}

export function clampWidth(width: number): number {
  return clamp(DRAWER, width);
}

export function clampSidebar(width: number): number {
  return clamp(SIDEBAR, width);
}

function usePanelWidth(limits: Limits): PanelWidth {
  const [width, setWidth] = useState(limits.initial);
  const startRef = useRef<DragStart | null>(null);

  useEffect(() => {
    function handleMove(event: MouseEvent) {
      const start = startRef.current;
      if (start === null) {
        return;
      }
      setWidth(clamp(limits, start.width + limits.sign * (event.clientX - start.x)));
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
  }, [limits]);

  const startResize = useCallback(
    (clientX: number) => {
      startRef.current = { x: clientX, width };
    },
    [width],
  );

  const nudge = useCallback(
    (delta: number) => {
      setWidth((current) => clamp(limits, current + delta));
    },
    [limits],
  );

  return { width, startResize, nudge };
}

export function useDrawerWidth(): PanelWidth {
  return usePanelWidth(DRAWER);
}

export function useSidebarWidth(): PanelWidth {
  return usePanelWidth(SIDEBAR);
}

import { useCallback, useEffect, useRef, useState } from 'react';
import type { DockSide } from './panels';

export const MIN_DRAWER_WIDTH = 320;
export const MAX_DRAWER_WIDTH = 1200;
export const DEFAULT_DRAWER_WIDTH = 560;

export const MIN_SIDEBAR_WIDTH = 160;
export const MAX_SIDEBAR_WIDTH = 560;
export const DEFAULT_SIDEBAR_WIDTH = 224;

export const MIN_DOCK_HEIGHT = 120;
export const MAX_DOCK_HEIGHT = 900;
export const DEFAULT_DOCK_HEIGHT = 240;

export const NUDGE_STEP = 32;

interface Limits {
  min: number;
  max: number;
  initial: number;
  sign: number;
  axis: 'x' | 'y';
}

const DRAWER: Limits = {
  min: MIN_DRAWER_WIDTH,
  max: MAX_DRAWER_WIDTH,
  initial: DEFAULT_DRAWER_WIDTH,
  sign: -1,
  axis: 'x',
};

const LEFT_DOCK: Limits = {
  min: MIN_DRAWER_WIDTH,
  max: MAX_DRAWER_WIDTH,
  initial: DEFAULT_DRAWER_WIDTH,
  sign: 1,
  axis: 'x',
};

const SIDEBAR: Limits = {
  min: MIN_SIDEBAR_WIDTH,
  max: MAX_SIDEBAR_WIDTH,
  initial: DEFAULT_SIDEBAR_WIDTH,
  sign: 1,
  axis: 'x',
};

const BOTTOM_DOCK: Limits = {
  min: MIN_DOCK_HEIGHT,
  max: MAX_DOCK_HEIGHT,
  initial: DEFAULT_DOCK_HEIGHT,
  sign: -1,
  axis: 'y',
};

interface DragStart {
  at: number;
  size: number;
}

export interface PanelSize {
  size: number;
  startResize: (client: number) => void;
  nudge: (delta: number) => void;
}

function clamp(limits: Limits, size: number): number {
  if (size < limits.min) {
    return limits.min;
  }
  if (size > limits.max) {
    return limits.max;
  }
  return size;
}

export function clampWidth(width: number): number {
  return clamp(DRAWER, width);
}

export function clampSidebar(width: number): number {
  return clamp(SIDEBAR, width);
}

export function clampDockHeight(height: number): number {
  return clamp(BOTTOM_DOCK, height);
}

function limitsFor(side: DockSide): Limits {
  if (side === 'left') {
    return LEFT_DOCK;
  }
  if (side === 'right') {
    return DRAWER;
  }
  return BOTTOM_DOCK;
}

function pointOf(limits: Limits, event: MouseEvent): number {
  if (limits.axis === 'x') {
    return event.clientX;
  }
  return event.clientY;
}

function usePanelSize(limits: Limits): PanelSize {
  const [size, setSize] = useState(limits.initial);
  const startRef = useRef<DragStart | null>(null);

  useEffect(() => {
    function handleMove(event: MouseEvent) {
      const start = startRef.current;
      if (start === null) {
        return;
      }
      if (event.buttons === 0) {
        startRef.current = null;
        return;
      }
      setSize(clamp(limits, start.size + limits.sign * (pointOf(limits, event) - start.at)));
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
    (client: number) => {
      startRef.current = { at: client, size };
    },
    [size],
  );

  const nudge = useCallback(
    (delta: number) => {
      setSize((current) => clamp(limits, current + limits.sign * delta));
    },
    [limits],
  );

  return { size, startResize, nudge };
}

export function useSidebarWidth(): PanelSize {
  return usePanelSize(SIDEBAR);
}

export function useDockSize(side: DockSide): PanelSize {
  return usePanelSize(limitsFor(side));
}

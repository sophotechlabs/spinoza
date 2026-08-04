import { useCallback, useEffect, useRef } from 'react';
import type { DockSide } from './panels';
import { usePanelsStore } from '../store/panels';

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

function sizeOf(limits: Limits, stored: number | null): number {
  if (stored === null) {
    return limits.initial;
  }
  return clamp(limits, stored);
}

function usePanelSize(
  limits: Limits,
  stored: number | null,
  apply: (size: number) => void,
): PanelSize {
  const size = sizeOf(limits, stored);
  const startRef = useRef<DragStart | null>(null);
  const sizeRef = useRef(size);
  sizeRef.current = size;

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
      apply(clamp(limits, start.size + limits.sign * (pointOf(limits, event) - start.at)));
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
  }, [limits, apply]);

  const startResize = useCallback((client: number) => {
    startRef.current = { at: client, size: sizeRef.current };
  }, []);

  const nudge = useCallback(
    (delta: number) => {
      apply(clamp(limits, sizeRef.current + limits.sign * delta));
    },
    [limits, apply],
  );

  return { size, startResize, nudge };
}

export function useSidebarWidth(): PanelSize {
  const stored = usePanelsStore((state) => state.sidebar);
  const apply = usePanelsStore((state) => state.resizeSidebar);
  return usePanelSize(SIDEBAR, stored, apply);
}

export function useDockSize(side: DockSide): PanelSize {
  const stored = usePanelsStore((state) => state.sizes[side]);
  const resize = usePanelsStore((state) => state.resize);
  const apply = useCallback(
    (size: number) => {
      resize(side, size);
    },
    [resize, side],
  );
  return usePanelSize(limitsFor(side), stored, apply);
}

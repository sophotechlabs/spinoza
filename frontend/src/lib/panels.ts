import { useCallback, useState } from 'react';

export type PanelId = 'overview' | 'yaml' | 'events' | 'logs' | 'metrics' | 'forwards' | 'terminal';

export type DockSide = 'left' | 'right' | 'bottom';

export type Placement = Record<PanelId, DockSide>;

export const PANEL_ORDER: PanelId[] = [
  'overview',
  'yaml',
  'events',
  'logs',
  'metrics',
  'forwards',
  'terminal',
];

export const PANEL_LABELS: Record<PanelId, string> = {
  overview: 'Overview',
  yaml: 'YAML',
  events: 'Events',
  logs: 'Logs',
  metrics: 'Metrics',
  forwards: 'Forwards',
  terminal: 'Terminal',
};

export const DOCK_SIDES: DockSide[] = ['left', 'right', 'bottom'];

export const SIDE_LABELS: Record<DockSide, string> = {
  left: 'left',
  right: 'right',
  bottom: 'bottom',
};

export const SIDE_GLYPHS: Record<DockSide, string> = {
  left: '⇤',
  right: '⇥',
  bottom: '⇩',
};

export const DEFAULT_PLACEMENT: Placement = {
  overview: 'right',
  yaml: 'right',
  events: 'right',
  logs: 'right',
  metrics: 'right',
  forwards: 'bottom',
  terminal: 'bottom',
};

export const PLACEMENT_KEY = 'spinoza.panels.v1';

function isPanelId(value: string): value is PanelId {
  return (PANEL_ORDER as string[]).includes(value);
}

function isDockSide(value: unknown): value is DockSide {
  if (typeof value !== 'string') {
    return false;
  }
  return (DOCK_SIDES as string[]).includes(value);
}

export function parsePlacement(raw: string | null): Placement {
  if (raw === null) {
    return { ...DEFAULT_PLACEMENT };
  }
  let parsed: unknown = null;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return { ...DEFAULT_PLACEMENT };
  }
  if (typeof parsed !== 'object') {
    return { ...DEFAULT_PLACEMENT };
  }
  if (parsed === null) {
    return { ...DEFAULT_PLACEMENT };
  }
  const stored = parsed as Record<string, unknown>;
  const placement = { ...DEFAULT_PLACEMENT };
  for (const [key, value] of Object.entries(stored)) {
    if (!isPanelId(key)) {
      continue;
    }
    if (!isDockSide(value)) {
      continue;
    }
    placement[key] = value;
  }
  return placement;
}

export function readPlacement(): Placement {
  try {
    return parsePlacement(window.localStorage.getItem(PLACEMENT_KEY));
  } catch {
    return { ...DEFAULT_PLACEMENT };
  }
}

export function writePlacement(placement: Placement): void {
  try {
    window.localStorage.setItem(PLACEMENT_KEY, JSON.stringify(placement));
  } catch {
    return;
  }
}

export function panelsOn(placement: Placement, side: DockSide): PanelId[] {
  return PANEL_ORDER.filter((id) => placement[id] === side);
}

export interface PlacementState {
  placement: Placement;
  move: (id: PanelId, side: DockSide) => void;
}

export function usePlacement(): PlacementState {
  const [placement, setPlacement] = useState<Placement>(readPlacement);

  const move = useCallback((id: PanelId, side: DockSide) => {
    setPlacement((current) => {
      if (current[id] === side) {
        return current;
      }
      const next = { ...current, [id]: side };
      writePlacement(next);
      return next;
    });
  }, []);

  return { placement, move };
}

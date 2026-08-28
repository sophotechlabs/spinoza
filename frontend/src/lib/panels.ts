import type { ObjectDetail, ReleaseRef, ResourceDescriptor } from './types';
import type { Capability } from './access';
import type { Selection } from './refs';
import type { PodTarget } from './pods';
import { readStored, writeStored } from './persist';
import { isGitopsApp } from './gitopsApp';

export type PanelId =
  | 'overview'
  | 'yaml'
  | 'events'
  | 'logs'
  | 'metrics'
  | 'forwards'
  | 'terminal'
  | 'release'
  | 'compare'
  | 'app';

export type DockSide = 'left' | 'right' | 'bottom';

export type Placement = Record<PanelId, DockSide>;

export interface PanelContext {
  selection: Selection | null;
  detail: ObjectDetail | null;
  pod: PodTarget | null;
  release: ReleaseRef | null;
  kind: ResourceDescriptor | null;
  namespace: string;
  refused: Partial<Record<Capability, string>>;
}

export interface PanelDescriptor {
  id: PanelId;
  label: string;
  defaultSide: DockSide;
  hint: string;
  enabled: (ctx: PanelContext) => boolean;
  refused?: (ctx: PanelContext) => string | null;
}

const SELECT_ROW = 'Select a row to inspect it';
const SELECT_POD = 'Select a pod to see this';
const SELECT_LOGGABLE = 'Select a pod, or a workload that owns some, to see logs';

const LOGGABLE_WORKLOADS = [
  'Deployment',
  'StatefulSet',
  'DaemonSet',
  'ReplicaSet',
  'Job',
  'ReplicationController',
];
const NOT_FOR_EVENTS = 'An event has no events of its own';

function isPod(detail: ObjectDetail | null): boolean {
  if (detail === null) {
    return false;
  }
  return detail.kind === 'Pod';
}

function hasSelection(ctx: PanelContext): boolean {
  return ctx.selection !== null;
}

function isPodPanel(ctx: PanelContext): boolean {
  return isPod(ctx.detail);
}

export function ownsPods(detail: ObjectDetail | null): boolean {
  if (detail === null) {
    return false;
  }
  return LOGGABLE_WORKLOADS.includes(detail.kind);
}

function hasLogs(ctx: PanelContext): boolean {
  return isPod(ctx.detail) || ownsPods(ctx.detail);
}

function takesEvents(ctx: PanelContext): boolean {
  if (!hasSelection(ctx)) {
    return false;
  }
  return ctx.detail?.kind !== 'Event';
}

function hasRelease(ctx: PanelContext): boolean {
  return ctx.release !== null;
}

function isGitopsApplier(ctx: PanelContext): boolean {
  if (ctx.detail === null) {
    return false;
  }
  return isGitopsApp(ctx.detail.apiVersion, ctx.detail.kind);
}

export const PANELS: PanelDescriptor[] = [
  {
    id: 'overview',
    label: 'Overview',
    defaultSide: 'right',
    hint: SELECT_ROW,
    enabled: hasSelection,
  },
  { id: 'yaml', label: 'YAML', defaultSide: 'right', hint: SELECT_ROW, enabled: hasSelection },
  {
    id: 'events',
    label: 'Events',
    defaultSide: 'right',
    hint: NOT_FOR_EVENTS,
    enabled: takesEvents,
  },
  {
    id: 'logs',
    label: 'Logs',
    defaultSide: 'right',
    hint: SELECT_LOGGABLE,
    enabled: hasLogs,
    refused: (ctx) => ctx.refused.logs ?? null,
  },
  { id: 'metrics', label: 'Metrics', defaultSide: 'right', hint: SELECT_POD, enabled: isPodPanel },
  {
    id: 'forwards',
    label: 'Forwards',
    defaultSide: 'bottom',
    hint: 'Nothing docked here yet',
    enabled: () => true,
  },
  {
    id: 'terminal',
    label: 'Terminal',
    defaultSide: 'bottom',
    hint: 'Nothing docked here yet',
    enabled: () => true,
  },
  {
    id: 'compare',
    label: 'Compare',
    defaultSide: 'bottom',
    hint: 'Open a kind, or select an object, to compare against another context',
    enabled: (ctx) => ctx.selection !== null || ctx.kind !== null,
  },
  {
    id: 'release',
    label: 'Release',
    defaultSide: 'bottom',
    hint: 'Select a Helm release to inspect it',
    enabled: hasRelease,
  },
  {
    id: 'app',
    label: 'Application',
    defaultSide: 'right',
    hint: 'Select an Argo application, a Kustomization or a HelmRelease',
    enabled: isGitopsApplier,
  },
];

const BY_ID = Object.fromEntries(PANELS.map((panel) => [panel.id, panel])) as Record<
  PanelId,
  PanelDescriptor
>;

export function panelById(id: PanelId): PanelDescriptor {
  return BY_ID[id];
}

export const PANEL_ORDER: PanelId[] = PANELS.map((panel) => panel.id);

export const DEFAULT_PLACEMENT: Placement = Object.fromEntries(
  PANELS.map((panel) => [panel.id, panel.defaultSide]),
) as Placement;

export const DOCK_SIDES: DockSide[] = ['left', 'right', 'bottom'];

export function tabId(id: PanelId): string {
  return `panel-tab-${id}`;
}

export function panelBodyId(id: PanelId): string {
  return `panel-body-${id}`;
}

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

export const PLACEMENT_KEY = 'spinoza.panels.v1';
const LAYOUT_KEY = 'spinoza.layout.v1';

export interface Layout {
  sizes: Record<DockSide, number | null>;
  collapsed: Record<DockSide, boolean>;
  active: Record<DockSide, PanelId | null>;
  sidebar: number | null;
}

export const DEFAULT_LAYOUT: Layout = {
  sizes: { left: null, right: null, bottom: null },
  collapsed: { left: false, right: false, bottom: false },
  active: { left: null, right: null, bottom: null },
  sidebar: null,
};

function isPanelId(value: unknown): value is PanelId {
  if (typeof value !== 'string') {
    return false;
  }
  return (PANEL_ORDER as string[]).includes(value);
}

function isDockSide(value: unknown): value is DockSide {
  if (typeof value !== 'string') {
    return false;
  }
  return (DOCK_SIDES as string[]).includes(value);
}

function readJson(raw: string | null): Record<string, unknown> | null {
  if (raw === null) {
    return null;
  }
  let parsed: unknown = null;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return null;
  }
  if (typeof parsed !== 'object') {
    return null;
  }
  if (parsed === null) {
    return null;
  }
  return parsed as Record<string, unknown>;
}

export function parsePlacement(raw: string | null): Placement {
  const stored = readJson(raw);
  const placement = { ...DEFAULT_PLACEMENT };
  if (stored === null) {
    return placement;
  }
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

function readSize(value: unknown): number | null {
  if (typeof value !== 'number') {
    return null;
  }
  if (!Number.isFinite(value)) {
    return null;
  }
  return value;
}

function sideRecord<T>(stored: Record<string, unknown> | undefined, read: (value: unknown) => T) {
  const out = {} as Record<DockSide, T>;
  for (const side of DOCK_SIDES) {
    out[side] = read(stored?.[side]);
  }
  return out;
}

function nested(stored: Record<string, unknown>, key: string): Record<string, unknown> | undefined {
  const value = stored[key];
  if (typeof value !== 'object') {
    return undefined;
  }
  if (value === null) {
    return undefined;
  }
  return value as Record<string, unknown>;
}

export function parseLayout(raw: string | null): Layout {
  const stored = readJson(raw);
  if (stored === null) {
    return {
      sizes: { ...DEFAULT_LAYOUT.sizes },
      collapsed: { ...DEFAULT_LAYOUT.collapsed },
      active: { ...DEFAULT_LAYOUT.active },
      sidebar: null,
    };
  }
  return {
    sizes: sideRecord(nested(stored, 'sizes'), readSize),
    collapsed: sideRecord(nested(stored, 'collapsed'), (value) => value === true),
    active: sideRecord(nested(stored, 'active'), (value) => {
      if (isPanelId(value)) {
        return value;
      }
      return null;
    }),
    sidebar: readSize(stored.sidebar),
  };
}

function read<T>(key: string, parse: (raw: string | null) => T): T {
  return parse(readStored(key));
}

function write(key: string, value: unknown): void {
  writeStored(key, JSON.stringify(value));
}

export function readPlacement(): Placement {
  return read(PLACEMENT_KEY, parsePlacement);
}

export function writePlacement(placement: Placement): void {
  write(PLACEMENT_KEY, placement);
}

export function readLayout(): Layout {
  return read(LAYOUT_KEY, parseLayout);
}

export function writeLayout(layout: Layout): void {
  write(LAYOUT_KEY, layout);
}

export function panelsOn(placement: Placement, side: DockSide): PanelId[] {
  return PANEL_ORDER.filter((id) => placement[id] === side);
}

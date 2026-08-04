import { useRef, useState } from 'react';
import type { DockSide, PanelId } from '../lib/panels';
import { DOCK_SIDES, SIDE_GLYPHS, SIDE_LABELS, panelBodyId, tabId } from '../lib/panels';
import { NUDGE_STEP, useDockSize } from '../lib/usePanelWidth';
import { usePanelsStore } from '../store/panels';

function arrowStep(key: string): number {
  if (key === 'ArrowRight' || key === 'ArrowDown') {
    return 1;
  }
  if (key === 'ArrowLeft' || key === 'ArrowUp') {
    return -1;
  }
  return 0;
}

function rovingIndex(selected: boolean, index: number, active: PanelId | null): number {
  if (selected) {
    return 0;
  }
  if (active === null && index === 0) {
    return 0;
  }
  return -1;
}

export interface PanelTab {
  id: PanelId;
  label: string;
  disabled: boolean;
  title: string;
}

function labelOf(tabs: PanelTab[], id: PanelId): string {
  const tab = tabs.find((one) => one.id === id);
  return tab?.label ?? '';
}

interface PanelHostProps {
  side: DockSide;
  tabs: PanelTab[];
  active: PanelId | null;
  onActivate: (id: PanelId) => void;
  onMove: (id: PanelId, side: DockSide) => void;
  hostRef: (element: HTMLDivElement | null) => void;
  emptyHint: string;
}

const DRAG_TYPE = 'application/x-spinoza-panel';

function tabClass(active: boolean, disabled: boolean): string {
  if (disabled) {
    return 'cursor-not-allowed border-b-2 border-transparent px-2 py-1.5 text-fg-faint';
  }
  if (active) {
    return 'border-b-2 border-edge-active px-2 py-1.5 text-fg-strong';
  }
  return 'border-b-2 border-transparent px-2 py-1.5 text-fg-muted hover:text-fg-soft';
}

function frameClass(side: DockSide): string {
  if (side === 'bottom') {
    return 'flex shrink-0 flex-col border-t border-edge bg-surface';
  }
  if (side === 'left') {
    return 'flex min-h-0 min-w-0 shrink-0 border-r border-edge bg-surface';
  }
  return 'flex min-h-0 min-w-0 shrink-0 border-l border-edge bg-surface';
}

function handleClass(side: DockSide): string {
  if (side === 'bottom') {
    return 'h-1 shrink-0 cursor-row-resize bg-handle hover:bg-handle-active';
  }
  return 'w-1 shrink-0 cursor-col-resize bg-handle hover:bg-handle-active';
}

function emptyClass(side: DockSide): string {
  if (side === 'bottom') {
    return 'h-2 shrink-0 border-t border-edge bg-surface hover:bg-surface-raised';
  }
  if (side === 'left') {
    return 'w-2 shrink-0 border-r border-edge bg-surface hover:bg-surface-raised';
  }
  return 'w-2 shrink-0 border-l border-edge bg-surface hover:bg-surface-raised';
}

function collapsedClass(side: DockSide): string {
  if (side === 'bottom') {
    return 'flex shrink-0 items-center border-t border-edge bg-surface px-1';
  }
  if (side === 'left') {
    return 'flex w-8 shrink-0 items-start justify-center border-r border-edge bg-surface pt-2';
  }
  return 'flex w-8 shrink-0 items-start justify-center border-l border-edge bg-surface pt-2';
}

function expandGlyph(side: DockSide): string {
  if (side === 'bottom') {
    return '▸';
  }
  if (side === 'left') {
    return '›';
  }
  return '‹';
}

function collapseGlyph(side: DockSide): string {
  if (side === 'bottom') {
    return '▾';
  }
  if (side === 'left') {
    return '‹';
  }
  return '›';
}

function panelFrom(event: React.DragEvent): PanelId | null {
  const raw = event.dataTransfer.getData(DRAG_TYPE);
  if (raw === '') {
    return null;
  }
  return raw as PanelId;
}

export default function PanelHost({
  side,
  tabs,
  active,
  onActivate,
  onMove,
  hostRef,
  emptyHint,
}: PanelHostProps) {
  const collapsed = usePanelsStore((state) => state.collapsed[side]);
  const collapse = usePanelsStore((state) => state.collapse);
  const [over, setOver] = useState(false);
  const insideRef = useRef(0);
  const { size, startResize, nudge } = useDockSize(side);

  function handleDragOver(event: React.DragEvent) {
    if (!event.dataTransfer.types.includes(DRAG_TYPE)) {
      return;
    }
    event.preventDefault();
    setOver(true);
  }

  function handleDragEnter(event: React.DragEvent) {
    if (!event.dataTransfer.types.includes(DRAG_TYPE)) {
      return;
    }
    insideRef.current += 1;
    setOver(true);
  }

  function handleDragLeave() {
    insideRef.current -= 1;
    if (insideRef.current > 0) {
      return;
    }
    insideRef.current = 0;
    setOver(false);
  }

  function handleDrop(event: React.DragEvent) {
    event.preventDefault();
    insideRef.current = 0;
    setOver(false);
    const id = panelFrom(event);
    if (id === null) {
      return;
    }
    onMove(id, side);
  }

  function handleResize(event: React.MouseEvent<HTMLButtonElement>) {
    event.preventDefault();
    if (side === 'bottom') {
      startResize(event.clientY);
      return;
    }
    startResize(event.clientX);
  }

  function handleResizeKey(event: React.KeyboardEvent<HTMLButtonElement>) {
    if (event.key === 'ArrowLeft') {
      event.preventDefault();
      nudge(-NUDGE_STEP);
      return;
    }
    if (event.key === 'ArrowRight') {
      event.preventDefault();
      nudge(NUDGE_STEP);
      return;
    }
    if (event.key === 'ArrowUp') {
      event.preventDefault();
      nudge(-NUDGE_STEP);
      return;
    }
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      nudge(NUDGE_STEP);
    }
  }

  const dropClass = over ? ' bg-surface-active' : '';

  function focusTab(index: number) {
    const wanted = tabs[index];
    document.getElementById(tabId(wanted.id))?.focus();
    if (wanted.disabled) {
      return;
    }
    onActivate(wanted.id);
  }

  function handleTabKey(event: React.KeyboardEvent<HTMLButtonElement>, index: number) {
    const step = arrowStep(event.key);
    if (step !== 0) {
      event.preventDefault();
      focusTab((index + step + tabs.length) % tabs.length);
      return;
    }
    if (event.key === 'Home') {
      event.preventDefault();
      focusTab(0);
      return;
    }
    if (event.key === 'End') {
      event.preventDefault();
      focusTab(tabs.length - 1);
    }
  }

  if (tabs.length === 0) {
    return (
      <div
        role="tablist"
        tabIndex={-1}
        aria-label={`Empty ${SIDE_LABELS[side]} dock`}
        onDragOver={handleDragOver}
        onDragEnter={handleDragEnter}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        className={emptyClass(side) + dropClass}
      />
    );
  }

  if (collapsed) {
    return (
      <div
        role="tablist"
        tabIndex={-1}
        aria-label={`Collapsed ${SIDE_LABELS[side]} dock`}
        onDragOver={handleDragOver}
        onDragEnter={handleDragEnter}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        className={collapsedClass(side) + dropClass}
      >
        <button
          type="button"
          aria-label={`Show the ${SIDE_LABELS[side]} dock`}
          onClick={() => {
            collapse(side, false);
          }}
          className="rounded px-1 py-0.5 text-xs text-fg-muted hover:bg-surface-raised hover:text-fg"
        >
          {expandGlyph(side)}
        </button>
      </div>
    );
  }

  const strip = (
    <div
      role="tablist"
      tabIndex={-1}
      aria-label={`${SIDE_LABELS[side]} dock`}
      onDragOver={handleDragOver}
      onDragEnter={handleDragEnter}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
      className={`flex shrink-0 flex-wrap items-center gap-1 border-b border-edge px-1 text-xs${dropClass}`}
    >
      <button
        type="button"
        aria-label={`Hide the ${SIDE_LABELS[side]} dock`}
        onClick={() => {
          collapse(side, true);
        }}
        className="px-1 py-1.5 text-fg-muted hover:text-fg"
      >
        {collapseGlyph(side)}
      </button>
      {tabs.map((tab, index) => (
        <button
          key={tab.id}
          id={tabId(tab.id)}
          type="button"
          role="tab"
          aria-selected={active === tab.id}
          aria-controls={panelBodyId(tab.id)}
          tabIndex={rovingIndex(active === tab.id, index, active)}
          draggable
          title={tab.title}
          aria-disabled={tab.disabled}
          onDragStart={(event) => {
            event.dataTransfer.setData(DRAG_TYPE, tab.id);
            event.dataTransfer.effectAllowed = 'move';
          }}
          onKeyDown={(event) => {
            handleTabKey(event, index);
          }}
          onClick={() => {
            if (tab.disabled) {
              return;
            }
            onActivate(tab.id);
          }}
          className={tabClass(active === tab.id, tab.disabled)}
        >
          {tab.label}
        </button>
      ))}
      {active !== null && (
        <span className="ml-auto flex items-center gap-0.5 pl-2">
          {DOCK_SIDES.filter((other) => other !== side).map((other) => (
            <button
              key={other}
              type="button"
              aria-label={`Move ${labelOf(tabs, active)} to the ${SIDE_LABELS[other]}`}
              onClick={() => {
                onMove(active, other);
              }}
              className="rounded px-1 text-fg-muted hover:bg-surface-active hover:text-fg"
            >
              {SIDE_GLYPHS[other]}
            </button>
          ))}
        </span>
      )}
    </div>
  );

  const resize = (
    <button
      type="button"
      aria-label={`Resize the ${SIDE_LABELS[side]} dock`}
      onMouseDown={handleResize}
      onKeyDown={handleResizeKey}
      className={handleClass(side)}
    />
  );

  const body = (
    <>
      {active === null && <div className="p-4 text-xs text-fg-muted">{emptyHint}</div>}
      <div ref={hostRef} className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden" />
    </>
  );

  if (side === 'bottom') {
    return (
      <div style={{ height: `${size}px` }} className={frameClass(side)}>
        {resize}
        {strip}
        {body}
      </div>
    );
  }

  const column = (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      {strip}
      {body}
    </div>
  );

  if (side === 'left') {
    return (
      <div style={{ width: `${size}px` }} className={frameClass(side)}>
        {column}
        {resize}
      </div>
    );
  }

  return (
    <div style={{ width: `${size}px` }} className={frameClass(side)}>
      {resize}
      {column}
    </div>
  );
}

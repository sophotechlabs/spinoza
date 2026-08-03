import { useState } from 'react';
import type { DockSide, PanelId } from '../lib/panels';
import { DOCK_SIDES, PANEL_LABELS, SIDE_GLYPHS, SIDE_LABELS } from '../lib/panels';
import { NUDGE_STEP, useDockSize } from '../lib/usePanelWidth';

export interface PanelTab {
  id: PanelId;
  disabled: boolean;
  title: string;
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
    return 'cursor-not-allowed border-b-2 border-transparent px-2 py-1.5 text-neutral-600';
  }
  if (active) {
    return 'border-b-2 border-neutral-300 px-2 py-1.5 text-neutral-100';
  }
  return 'border-b-2 border-transparent px-2 py-1.5 text-neutral-400 hover:text-neutral-300';
}

function frameClass(side: DockSide): string {
  if (side === 'bottom') {
    return 'flex shrink-0 flex-col border-t border-neutral-800 bg-neutral-950';
  }
  if (side === 'left') {
    return 'flex min-h-0 min-w-0 shrink-0 border-r border-neutral-800 bg-neutral-950';
  }
  return 'flex min-h-0 min-w-0 shrink-0 border-l border-neutral-800 bg-neutral-950';
}

function handleClass(side: DockSide): string {
  if (side === 'bottom') {
    return 'h-1 shrink-0 cursor-row-resize bg-neutral-500 hover:bg-neutral-300';
  }
  return 'w-1 shrink-0 cursor-col-resize bg-neutral-500 hover:bg-neutral-300';
}

function emptyClass(side: DockSide): string {
  if (side === 'bottom') {
    return 'h-2 shrink-0 border-t border-neutral-800 bg-neutral-950 hover:bg-neutral-900';
  }
  if (side === 'left') {
    return 'w-2 shrink-0 border-r border-neutral-800 bg-neutral-950 hover:bg-neutral-900';
  }
  return 'w-2 shrink-0 border-l border-neutral-800 bg-neutral-950 hover:bg-neutral-900';
}

function collapsedClass(side: DockSide): string {
  if (side === 'bottom') {
    return 'flex shrink-0 items-center border-t border-neutral-800 bg-neutral-950 px-1';
  }
  if (side === 'left') {
    return 'flex w-8 shrink-0 items-start justify-center border-r border-neutral-800 bg-neutral-950 pt-2';
  }
  return 'flex w-8 shrink-0 items-start justify-center border-l border-neutral-800 bg-neutral-950 pt-2';
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
  const [collapsed, setCollapsed] = useState(false);
  const [over, setOver] = useState(false);
  const { size, startResize, nudge } = useDockSize(side);

  function handleDragOver(event: React.DragEvent) {
    if (!event.dataTransfer.types.includes(DRAG_TYPE)) {
      return;
    }
    event.preventDefault();
    setOver(true);
  }

  function handleDragLeave() {
    setOver(false);
  }

  function handleDrop(event: React.DragEvent) {
    event.preventDefault();
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
      nudge(NUDGE_STEP);
      return;
    }
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      nudge(-NUDGE_STEP);
    }
  }

  const dropClass = over ? ' bg-neutral-800' : '';

  if (tabs.length === 0) {
    return (
      <div
        role="tablist"
        tabIndex={-1}
        aria-label={`Empty ${SIDE_LABELS[side]} dock`}
        onDragOver={handleDragOver}
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
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        className={collapsedClass(side) + dropClass}
      >
        <button
          type="button"
          aria-label={`Show the ${SIDE_LABELS[side]} dock`}
          onClick={() => {
            setCollapsed(false);
          }}
          className="rounded px-1 py-0.5 text-xs text-neutral-400 hover:bg-neutral-900 hover:text-neutral-200"
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
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
      className={`flex shrink-0 flex-wrap items-center gap-1 border-b border-neutral-800 px-1 text-xs${dropClass}`}
    >
      <button
        type="button"
        aria-label={`Hide the ${SIDE_LABELS[side]} dock`}
        onClick={() => {
          setCollapsed(true);
        }}
        className="px-1 py-1.5 text-neutral-400 hover:text-neutral-200"
      >
        {collapseGlyph(side)}
      </button>
      {tabs.map((tab) => (
        <button
          key={tab.id}
          type="button"
          role="tab"
          aria-selected={active === tab.id}
          draggable
          title={tab.title}
          aria-disabled={tab.disabled}
          onDragStart={(event) => {
            event.dataTransfer.setData(DRAG_TYPE, tab.id);
            event.dataTransfer.effectAllowed = 'move';
          }}
          onClick={() => {
            if (tab.disabled) {
              return;
            }
            onActivate(tab.id);
          }}
          className={tabClass(active === tab.id, tab.disabled)}
        >
          {PANEL_LABELS[tab.id]}
        </button>
      ))}
      {active !== null && (
        <span className="ml-auto flex items-center gap-0.5 pl-2">
          {DOCK_SIDES.filter((other) => other !== side).map((other) => (
            <button
              key={other}
              type="button"
              aria-label={`Move ${PANEL_LABELS[active]} to the ${SIDE_LABELS[other]}`}
              onClick={() => {
                onMove(active, other);
              }}
              className="rounded px-1 text-neutral-400 hover:bg-neutral-800 hover:text-neutral-200"
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
      {active === null && <div className="p-4 text-xs text-neutral-400">{emptyHint}</div>}
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

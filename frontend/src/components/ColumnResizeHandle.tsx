import { useRef } from 'react';
import type { KeyboardEvent, PointerEvent } from 'react';

const COLUMN_NUDGE_STEP = 16;

interface ColumnResizeHandleProps {
  column: string;
  size: number;
  min: number;
  onSize: (size: number) => void;
  onReset: () => void;
}

interface Drag {
  pointer: number;
  startX: number;
  startSize: number;
}

export default function ColumnResizeHandle({
  column,
  size,
  min,
  onSize,
  onReset,
}: ColumnResizeHandleProps) {
  const drag = useRef<Drag | null>(null);

  function handleKey(event: KeyboardEvent<HTMLButtonElement>) {
    if (event.key === 'ArrowLeft') {
      event.preventDefault();
      onSize(Math.max(min, size - COLUMN_NUDGE_STEP));
      return;
    }
    if (event.key === 'ArrowRight') {
      event.preventDefault();
      onSize(size + COLUMN_NUDGE_STEP);
      return;
    }
    if (event.key === 'Home') {
      event.preventDefault();
      onReset();
    }
  }

  function handlePointerDown(event: PointerEvent<HTMLButtonElement>) {
    if (event.button !== 0) {
      return;
    }
    event.preventDefault();
    drag.current = { pointer: event.pointerId, startX: event.clientX, startSize: size };
    if (typeof event.currentTarget.setPointerCapture === 'function') {
      event.currentTarget.setPointerCapture(event.pointerId);
    }
  }

  function handlePointerMove(event: PointerEvent<HTMLButtonElement>) {
    const started = drag.current;
    if (started?.pointer !== event.pointerId) {
      return;
    }
    onSize(Math.max(min, started.startSize + event.clientX - started.startX));
  }

  function handleLostCapture() {
    drag.current = null;
  }

  function handlePointerUp(event: PointerEvent<HTMLButtonElement>) {
    if (drag.current === null) {
      return;
    }
    drag.current = null;
    if (typeof event.currentTarget.releasePointerCapture === 'function') {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
  }

  return (
    <button
      type="button"
      aria-label={`Resize the ${column} column`}
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={handlePointerUp}
      onPointerCancel={handlePointerUp}
      onLostPointerCapture={handleLostCapture}
      onKeyDown={handleKey}
      className="absolute top-0 right-0 h-full w-2 cursor-col-resize touch-none bg-grip opacity-0 select-none hover:opacity-100 focus-visible:opacity-100"
    />
  );
}

import type { KeyboardEvent, MouseEvent, TouchEvent } from 'react';

const COLUMN_NUDGE_STEP = 16;

interface ColumnResizeHandleProps {
  column: string;
  size: number;
  min: number;
  onSize: (size: number) => void;
  onReset: () => void;
  onMouseDown: (event: MouseEvent) => void;
  onTouchStart: (event: TouchEvent) => void;
}

export default function ColumnResizeHandle({
  column,
  size,
  min,
  onSize,
  onReset,
  onMouseDown,
  onTouchStart,
}: ColumnResizeHandleProps) {
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

  return (
    <button
      type="button"
      aria-label={`Resize the ${column} column`}
      onMouseDown={onMouseDown}
      onTouchStart={onTouchStart}
      onKeyDown={handleKey}
      className="absolute top-0 right-0 h-full w-1 cursor-col-resize touch-none bg-grip opacity-0 select-none hover:opacity-100 focus-visible:opacity-100"
    />
  );
}

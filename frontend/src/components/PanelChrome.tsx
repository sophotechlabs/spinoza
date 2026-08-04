import type { ReactNode } from 'react';
import type { ObjectRef } from '../lib/types';

interface PanelChromeProps {
  target: ObjectRef | null;
  onClose: () => void;
  children: ReactNode;
}

export default function PanelChrome({ target, onClose, children }: PanelChromeProps) {
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      {target !== null && (
        <div className="flex shrink-0 items-center gap-2 border-b border-edge px-3 py-1.5">
          <span className="shrink-0 text-[11px] text-fg-muted">{target.resource}</span>
          <span className="truncate text-xs font-semibold text-fg-strong">{target.name}</span>
          <button
            type="button"
            onClick={onClose}
            className="ml-auto shrink-0 rounded border border-edge-strong px-1.5 text-xs text-fg-soft hover:bg-surface-active"
          >
            Close
          </button>
        </div>
      )}
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">{children}</div>
    </div>
  );
}

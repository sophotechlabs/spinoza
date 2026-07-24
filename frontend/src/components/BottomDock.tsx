import { useState } from 'react';

const tabs = ['Logs', 'Terminal'];

function chevron(open: boolean): string {
  if (open) {
    return '▾';
  }
  return '▸';
}

export default function BottomDock() {
  const [open, setOpen] = useState(false);

  function toggle() {
    setOpen((value) => !value);
  }

  return (
    <div className="shrink-0 border-t border-neutral-800 bg-neutral-900 text-xs">
      <div className="flex items-center">
        <button
          type="button"
          onClick={toggle}
          className="flex items-center gap-1.5 px-3 py-1.5 text-neutral-300 hover:bg-neutral-800"
        >
          <span>{chevron(open)}</span>
          <span>Panel</span>
        </button>
        {open && (
          <div className="flex items-center gap-1 border-l border-neutral-800 pl-2">
            {tabs.map((tab) => (
              <button
                key={tab}
                type="button"
                disabled
                className="cursor-not-allowed px-2 py-1 text-neutral-600"
              >
                {tab}
              </button>
            ))}
          </div>
        )}
      </div>
      {open && (
        <div className="h-40 border-t border-neutral-800 p-3 text-neutral-600">
          No output.
        </div>
      )}
    </div>
  );
}

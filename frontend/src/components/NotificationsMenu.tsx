import { useRef } from 'react';
import type { ObjectRef } from '../lib/types';
import { ICON_CONTROL } from '../lib/controls';
import NotificationsPanel from './NotificationsPanel';

interface NotificationsMenuProps {
  onSelectObject: (ref: ObjectRef) => void;
}

function Bell() {
  return (
    <svg
      viewBox="0 0 24 24"
      aria-hidden="true"
      focusable="false"
      className="h-4 w-4"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M18 8.5a6 6 0 1 0-12 0c0 4.5-1.5 6-2 6.8h16c-.5-.8-2-2.3-2-6.8Z" />
      <path d="M10 18.5a2 2 0 0 0 4 0" />
    </svg>
  );
}

export default function NotificationsMenu({ onSelectObject }: NotificationsMenuProps) {
  const ref = useRef<HTMLDetailsElement | null>(null);

  function handleSelectObject(target: ObjectRef) {
    const menu = ref.current;
    if (menu !== null) {
      menu.open = false;
    }
    onSelectObject(target);
  }

  return (
    <details ref={ref} className="relative">
      <summary
        aria-label="Notifications"
        title="What has happened on this cluster"
        className={`${ICON_CONTROL} cursor-pointer list-none border-edge-strong text-fg hover:bg-surface-active [&::-webkit-details-marker]:hidden`}
      >
        <Bell />
      </summary>
      <div className="absolute right-0 z-30 mt-1 flex max-h-[60vh] w-[28rem] flex-col overflow-hidden rounded border border-edge-strong bg-surface-raised shadow">
        <NotificationsPanel onSelectObject={handleSelectObject} />
      </div>
    </details>
  );
}

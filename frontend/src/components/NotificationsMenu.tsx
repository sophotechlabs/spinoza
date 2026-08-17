import { useRef } from 'react';
import type { ObjectRef } from '../lib/types';
import { ICON_CONTROL } from '../lib/controls';
import NotificationsPanel from './NotificationsPanel';
import { BellIcon } from './icons';

interface NotificationsMenuProps {
  onSelectObject: (ref: ObjectRef) => void;
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
        title="What happened on this cluster"
        className={`${ICON_CONTROL} cursor-pointer list-none border-edge-strong text-fg hover:bg-surface-active [&::-webkit-details-marker]:hidden`}
      >
        <BellIcon />
      </summary>
      <div className="absolute right-0 z-30 mt-1 flex max-h-[60vh] w-[28rem] flex-col overflow-hidden rounded border border-edge-strong bg-surface-raised shadow">
        <NotificationsPanel onSelectObject={handleSelectObject} />
      </div>
    </details>
  );
}

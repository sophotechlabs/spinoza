import { useRef } from 'react';
import type { ObjectRef } from '../lib/types';
import NotificationsPanel from './NotificationsPanel';

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
        title="What has happened on this cluster"
        className="cursor-pointer rounded border border-edge-strong px-1.5 py-0.5 text-base leading-none text-fg hover:bg-surface-active"
      >
        🔔
      </summary>
      <div className="absolute right-0 z-30 mt-1 flex max-h-[60vh] w-[28rem] flex-col overflow-hidden rounded border border-edge-strong bg-surface-raised shadow">
        <NotificationsPanel onSelectObject={handleSelectObject} />
      </div>
    </details>
  );
}

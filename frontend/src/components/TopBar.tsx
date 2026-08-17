import type { ConnectionStatus } from '../lib/feed';
import type { ObjectRef } from '../lib/types';
import { CONTROL, ICON_CONTROL } from '../lib/controls';
import { paletteChordLabel } from '../lib/hotkeys';
import ContextPicker from './ContextPicker';
import NotificationsMenu from './NotificationsMenu';
import ProtectionToggle from './ProtectionToggle';
import ViewSwitch from './ViewSwitch';
import { ReconnectIcon } from './icons';
import { useNamespaces } from '../lib/namespaces';
import { ALL, useNamespaceStore } from '../store/namespace';
import Wordmark from './Wordmark';

interface TopBarProps {
  status: ConnectionStatus;
  onReconnect?: () => void;
  onContextChanged?: () => void;
  onOpenPalette?: () => void;
  onOpenSettings?: () => void;
  onSelectObject?: (ref: ObjectRef) => void;
  onLeftForDesktop?: () => void;
}

function statusColor(status: ConnectionStatus): string {
  if (status === 'connected') {
    return 'bg-ok-solid';
  }
  if (status === 'connecting') {
    return 'bg-warn-solid';
  }
  return 'bg-error-solid';
}

export default function TopBar({
  status,
  onReconnect,
  onContextChanged,
  onOpenPalette,
  onOpenSettings,
  onSelectObject,
  onLeftForDesktop,
}: TopBarProps) {
  const namespace = useNamespaceStore((state) => state.namespace);
  const names = useNamespaceStore((state) => state.names);
  const choose = useNamespaceStore((state) => state.choose);
  useNamespaces();

  function handlePalette() {
    if (onOpenPalette) {
      onOpenPalette();
    }
  }

  function handleSettings() {
    if (onOpenSettings) {
      onOpenSettings();
    }
  }

  function handleSelectObject(target: ObjectRef) {
    if (onSelectObject) {
      onSelectObject(target);
    }
  }

  function handleLeftForDesktop() {
    if (onLeftForDesktop) {
      onLeftForDesktop();
    }
  }

  function handleReconnect() {
    if (onReconnect) {
      onReconnect();
    }
  }

  function handleContextChanged() {
    if (onContextChanged) {
      onContextChanged();
      return;
    }
    handleReconnect();
  }

  return (
    <header className="flex h-10 shrink-0 items-center gap-4 border-b border-edge bg-surface-raised px-3 text-xs">
      <Wordmark />
      <ContextPicker onSwitched={handleContextChanged} />
      <ProtectionToggle />
      <span
        role="status"
        aria-label={`The cluster feed is ${status}`}
        title={`The cluster feed is ${status}`}
        className={`${ICON_CONTROL} border-transparent`}
      >
        <span
          data-testid="connection-dot"
          className={`h-2 w-2 rounded-full ${statusColor(status)}`}
        />
      </span>
      <button
        type="button"
        aria-label="Reconnect"
        title="Reconnect to the cluster"
        onClick={handleReconnect}
        className={`${ICON_CONTROL} border-edge-strong text-fg hover:bg-surface-active`}
      >
        <ReconnectIcon />
      </button>
      <select
        aria-label="Namespace"
        title="The namespace the resource list shows"
        value={namespace}
        onChange={(event) => {
          choose(event.target.value);
        }}
        className={`${CONTROL} max-w-48 border-edge-strong bg-surface-raised text-fg-soft`}
      >
        <option value={ALL}>All namespaces</option>
        {names.map((name) => (
          <option key={name} value={name}>
            {name}
          </option>
        ))}
      </select>
      <div className="ml-auto flex items-center gap-3">
        <button
          type="button"
          onClick={handlePalette}
          title="Search resources, views and recent objects"
          className={`${CONTROL} border-edge-strong text-fg-soft hover:bg-surface-active`}
        >
          Search <span className="text-fg-muted">{paletteChordLabel()}</span>
        </button>
        <ViewSwitch onLeft={handleLeftForDesktop} />
        <NotificationsMenu onSelectObject={handleSelectObject} />
        <button
          type="button"
          aria-label="Settings"
          title="Settings"
          onClick={handleSettings}
          className={`${ICON_CONTROL} border-edge-strong text-xl leading-none text-fg hover:bg-surface-active`}
        >
          ⚙
        </button>
      </div>
    </header>
  );
}

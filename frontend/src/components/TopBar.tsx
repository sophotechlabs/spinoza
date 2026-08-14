import type { ConnectionStatus } from '../lib/feed';
import type { View } from '../lib/types';
import ContextPicker from './ContextPicker';
import ProtectionToggle from './ProtectionToggle';

interface TopBarProps {
  status: ConnectionStatus;
  view?: View;
  onReconnect?: () => void;
  onContextChanged?: () => void;
  onOpenPalette?: () => void;
  onOpenSettings?: () => void;
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
  view,
  onReconnect,
  onContextChanged,
  onOpenPalette,
  onOpenSettings,
}: TopBarProps) {
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
      <ContextPicker onSwitched={handleContextChanged} />
      <ProtectionToggle />
      <span className="text-fg-muted">/</span>
      <span className="text-fg-soft">all namespaces</span>
      {view !== undefined && (
        <span className="rounded border border-edge-strong px-1.5 py-0.5 text-fg-soft">{view}</span>
      )}
      <div className="ml-auto flex items-center gap-3">
        <button
          type="button"
          onClick={handlePalette}
          title="Search resources, views and recent objects"
          className="rounded border border-edge-strong px-2 py-0.5 text-fg-soft hover:bg-surface-active"
        >
          Search <span className="text-fg-muted">Ctrl K</span>
        </button>
        <span className="flex items-center gap-1.5 text-fg-muted">
          <span
            data-testid="connection-dot"
            className={`h-2 w-2 rounded-full ${statusColor(status)}`}
          />
          {status}
        </span>
        <button
          type="button"
          onClick={handleReconnect}
          className="rounded border border-edge-strong px-2 py-0.5 text-fg hover:bg-surface-active"
        >
          Reconnect
        </button>
        <button
          type="button"
          aria-label="Settings"
          title="Settings"
          onClick={handleSettings}
          className="rounded border border-edge-strong px-1.5 py-0.5 text-base leading-none text-fg hover:bg-surface-active"
        >
          ⚙
        </button>
      </div>
    </header>
  );
}

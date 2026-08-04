import { useState } from 'react';
import type { ConnectionStatus } from '../lib/feed';
import SettingsDialog from './SettingsDialog';
import type { View } from '../lib/types';
import ContextPicker from './ContextPicker';
import { SYSTEM } from '../lib/theme';
import { useThemePreference, useThemeStore, useThemes } from '../store/theme';

interface TopBarProps {
  status: ConnectionStatus;
  view?: View;
  onReconnect?: () => void;
  onContextChanged?: () => void;
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

export default function TopBar({ status, view, onReconnect, onContextChanged }: TopBarProps) {
  const preference = useThemePreference();
  const themes = useThemes();
  const setPreference = useThemeStore((state) => state.setPreference);
  const [settingsOpen, setSettingsOpen] = useState(false);

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
      <span className="text-fg-muted">/</span>
      <span className="text-fg-soft">all namespaces</span>
      {view !== undefined && (
        <span className="rounded border border-edge-strong px-1.5 py-0.5 text-fg-soft">{view}</span>
      )}
      <div className="ml-auto flex items-center gap-3">
        <select
          aria-label="Theme"
          value={preference}
          onChange={(event) => {
            setPreference(event.target.value);
          }}
          className="rounded border border-edge-strong bg-surface-raised px-1 py-0.5 text-fg"
        >
          {themes.map((theme) => (
            <option key={theme.id} value={theme.id}>
              {theme.name}
            </option>
          ))}
          <option value={SYSTEM}>System</option>
        </select>
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
          onClick={() => {
            setSettingsOpen(true);
          }}
          className="rounded border border-edge-strong px-2 py-0.5 text-fg hover:bg-surface-active"
        >
          ⚙
        </button>
      </div>
      <SettingsDialog
        open={settingsOpen}
        onClose={() => {
          setSettingsOpen(false);
        }}
      />
    </header>
  );
}

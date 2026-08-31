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
import { ALL, useNamespace, useNamespaceNames, useNamespaceStore } from '../store/namespace';
import {
  useClusterReachable,
  useClusterUnreachableReason,
  useClusterWobbling,
} from '../store/clusterHealth';
import UserMenu from './UserMenu';
import Wordmark from './Wordmark';
import { useClusterMode } from '../store/identity';

interface TopBarProps {
  status: ConnectionStatus;
  scoped?: boolean | null;
  onReconnect?: () => void;
  onContextChanged?: () => void;
  onOpenPalette?: () => void;
  onOpenSettings?: () => void;
  onSelectObject?: (ref: ObjectRef) => void;
  onLeftForDesktop?: () => void;
}

function pickerTitle(scoped: boolean | null): string {
  if (scoped === false) {
    return 'This kind is not in a namespace';
  }
  return 'The namespace the resource list shows';
}

function statusColor(
  status: ConnectionStatus,
  clusterReachable: boolean,
  wobbling: boolean,
): string {
  if (status === 'connecting') {
    return 'bg-warn-solid';
  }
  if (status !== 'connected') {
    return 'bg-error-solid';
  }
  if (!clusterReachable) {
    return 'bg-error-solid';
  }
  if (wobbling) {
    return 'bg-warn-solid';
  }
  return 'bg-ok-solid';
}

function statusLabel(
  status: ConnectionStatus,
  clusterReachable: boolean,
  wobbling: boolean,
  reason: string,
): string {
  if (status === 'connected' && clusterReachable && wobbling) {
    if (reason === '') {
      return 'The cluster missed a ping; still showing what it last said';
    }
    return `The cluster missed a ping: ${reason}`;
  }
  if (status === 'connected' && !clusterReachable) {
    if (reason === '') {
      return 'The cluster is not answering; what is on screen is the last thing it said';
    }
    return `The cluster is not answering: ${reason}`;
  }
  return `The cluster feed is ${status}`;
}

export default function TopBar({
  status,
  scoped = null,
  onReconnect,
  onContextChanged,
  onOpenPalette,
  onOpenSettings,
  onSelectObject,
  onLeftForDesktop,
}: TopBarProps) {
  const served = useClusterMode();
  const clusterReachable = useClusterReachable();
  const unreachableReason = useClusterUnreachableReason();
  const wobbling = useClusterWobbling();
  const namespace = useNamespace();
  const names = useNamespaceNames();
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
      {!served && <ContextPicker onSwitched={handleContextChanged} />}
      <div className="flex items-center gap-1.5">
        <ProtectionToggle />
        <span
          role="status"
          aria-label={statusLabel(status, clusterReachable, wobbling, unreachableReason)}
          title={statusLabel(status, clusterReachable, wobbling, unreachableReason)}
          className={`${ICON_CONTROL} border-edge-strong`}
        >
          <span
            data-testid="connection-dot"
            className={`h-2 w-2 rounded-full ${statusColor(status, clusterReachable, wobbling)}`}
          />
        </span>
        {status === 'connected' && !clusterReachable && (
          <span className="shrink-0 text-error">cluster not answering</span>
        )}
        <button
          type="button"
          aria-label="Reconnect"
          title="Reconnect to the cluster"
          onClick={handleReconnect}
          className={`${ICON_CONTROL} border-edge-strong text-fg hover:bg-surface-active`}
        >
          <ReconnectIcon />
        </button>
      </div>
      <select
        aria-label="Namespace"
        title={pickerTitle(scoped)}
        disabled={scoped === false}
        value={namespace}
        onChange={(event) => {
          choose(event.target.value);
        }}
        className={`${CONTROL} max-w-48 border-edge-strong bg-surface-raised text-fg-soft disabled:cursor-not-allowed disabled:text-fg-muted`}
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
        {!served && <ViewSwitch onLeft={handleLeftForDesktop} />}
        <NotificationsMenu onSelectObject={handleSelectObject} />
        <UserMenu />
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

import type { FeedStatus } from '../lib/feed';

interface TopBarProps {
  status: FeedStatus['status'];
  onReconnect?: () => void;
}

function statusColor(status: FeedStatus['status']): string {
  if (status === 'connected') {
    return 'bg-green-500';
  }
  if (status === 'connecting') {
    return 'bg-yellow-500';
  }
  return 'bg-red-500';
}

export default function TopBar({ status, onReconnect }: TopBarProps) {
  function handleReconnect() {
    if (onReconnect) {
      onReconnect();
    }
  }

  return (
    <header className="flex h-10 shrink-0 items-center gap-4 border-b border-neutral-800 bg-neutral-900 px-3 text-xs">
      <span className="font-semibold text-neutral-100">current-context</span>
      <span className="text-neutral-500">/</span>
      <span className="text-neutral-300">all namespaces</span>
      <div className="ml-auto flex items-center gap-3">
        <span className="flex items-center gap-1.5 text-neutral-400">
          <span className={`h-2 w-2 rounded-full ${statusColor(status)}`} />
          {status}
        </span>
        <button
          type="button"
          onClick={handleReconnect}
          className="rounded border border-neutral-700 px-2 py-0.5 text-neutral-200 hover:bg-neutral-800"
        >
          Reconnect
        </button>
      </div>
    </header>
  );
}

import type { ConnectionStatus } from '../lib/feed';
import { offline } from '../lib/feed';
import { useSessionExpired } from '../store/session';

interface ConnectionBannerProps {
  status: ConnectionStatus;
  attempt: number;
  onReconnect: () => void;
}

function detail(attempt: number): string {
  if (attempt === 0) {
    return 'Reconnecting';
  }
  return `Reconnecting, attempt ${String(attempt)}.`;
}

export default function ConnectionBanner({ status, attempt, onReconnect }: ConnectionBannerProps) {
  const expired = useSessionExpired();

  if (expired) {
    return (
      <div
        role="status"
        className="flex shrink-0 items-baseline gap-2 border-b border-warn-line bg-warn-tint/40 px-3 py-1.5 text-xs text-warn-strong"
      >
        <span className="shrink-0 font-semibold text-warn">
          This page belongs to an earlier run of spinoza.
        </span>
        <span className="min-w-0 flex-1 truncate">
          Every run prints a new token. Open the URL the running one printed, and this page will
          work again.
        </span>
      </div>
    );
  }

  if (!offline(status, attempt)) {
    return null;
  }
  return (
    <div
      role="status"
      className="flex shrink-0 items-baseline gap-2 border-b border-error-line bg-error-tint/40 px-3 py-1.5 text-xs text-error-strong"
    >
      <span className="shrink-0 font-semibold text-error">
        The live connection dropped. What follows is the last the cluster sent.
      </span>
      <span className="min-w-0 flex-1 truncate">{detail(attempt)}</span>
      <button
        type="button"
        onClick={onReconnect}
        className="shrink-0 rounded border border-error-line-strong px-1.5 py-0.5 text-error-contrast hover:bg-error-tint-strong"
      >
        Reconnect now
      </button>
    </div>
  );
}

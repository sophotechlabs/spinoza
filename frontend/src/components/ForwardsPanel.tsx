import { useState } from 'react';
import type { ReactNode } from 'react';
import { refreshForwards, stopForward, useForwardPolling } from '../lib/portForward';
import { useForwardsStore } from '../store/forwards';
import { notifyError, notifyOk } from '../store/toasts';
import StaleBanner from './StaleBanner';
import CopyButton from './CopyButton';

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'could not stop the forward';
}

function stateColor(state: string): string {
  if (state === 'failed') {
    return 'text-error';
  }
  return 'text-ok';
}

interface ForwardsPanelProps {
  active?: boolean;
}

export default function ForwardsPanel({ active = true }: ForwardsPanelProps) {
  const forwards = useForwardsStore((state) => state.forwards);
  const [error, setError] = useState<string | null>(null);
  const poll = useForwardPolling(active);

  async function stop(id: string) {
    setError(null);
    try {
      await stopForward(id);
      notifyOk('Forward stopped');
      await refreshForwards();
    } catch (err: unknown) {
      const message = errorMessage(err);
      setError(message);
      notifyError(`Stopping the forward: ${message}`);
    }
  }

  let notice: ReactNode = null;
  if (poll.error !== null) {
    notice = <StaleBanner what="The forward list" message={poll.error} onRetry={poll.reload} />;
  }

  if (forwards.length === 0) {
    return (
      <div className="flex min-h-0 flex-col">
        {notice}
        <div className="p-3 text-fg-muted">
          No active forwards. Open a Pod or Service and forward a port.
        </div>
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-col">
      {notice}
      <div className="p-2">
        {error !== null && <p className="mb-1.5 text-error">{error}</p>}
        {forwards.map((forward) => (
          <div key={forward.id} className="flex items-center gap-2 border-b border-edge py-1">
            <span className={stateColor(forward.state)}>●</span>
            <span className="truncate text-fg-soft">
              {forward.kind.toLowerCase()}/{forward.namespace}/{forward.name}
            </span>
            {forward.state === 'failed' && (
              <span className="truncate text-error" title={forward.error}>
                {forward.error}
              </span>
            )}
            {forward.state !== 'failed' && (
              <a
                href={`http://127.0.0.1:${forward.localPort}`}
                target="_blank"
                rel="noreferrer"
                className="text-fg-strong hover:underline"
              >
                127.0.0.1:{forward.localPort}
              </a>
            )}
            {forward.state !== 'failed' && (
              <CopyButton
                what={`${forward.name} forward url`}
                text={`http://127.0.0.1:${String(forward.localPort)}`}
              />
            )}
            <span className="text-fg-muted">→ {forward.remotePort}</span>
            <button
              type="button"
              onClick={() => void stop(forward.id)}
              className="ml-auto rounded border border-edge-strong px-1.5 text-fg-soft hover:bg-surface-active"
            >
              Stop
            </button>
          </div>
        ))}
      </div>
    </div>
  );
}

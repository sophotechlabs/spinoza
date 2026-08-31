import { useState } from 'react';
import type { ReactNode } from 'react';
import {
  NO_FORWARDS_WHEN_SERVED,
  refreshForwards,
  stopForward,
  useForwardPolling,
} from '../lib/portForward';
import { forwardURL, openExternal } from '../lib/openExternal';
import { useForwards } from '../store/forwards';
import { useClusterMode } from '../store/identity';
import { notifyError, notifyOk } from '../store/toasts';
import StaleBanner from './StaleBanner';
import CopyButton from './CopyButton';
import Announce from './Announce';

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
  const served = useClusterMode();
  const forwards = useForwards();
  const [error, setError] = useState<string | null>(null);
  const poll = useForwardPolling(active && !served);

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

  if (served) {
    return (
      <div className="flex min-h-0 flex-col">
        <div className="p-3 text-fg-muted">{NO_FORWARDS_WHEN_SERVED}</div>
      </div>
    );
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
        <Announce message={error} urgent className="mb-1.5 text-error" />
        {forwards.map((forward) => (
          <div key={forward.id} className="flex items-center gap-2 border-b border-edge py-1">
            <span role="img" aria-label={forward.state} className={stateColor(forward.state)}>
              ●
            </span>
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
                href={forwardURL(forward.localPort)}
                target="_blank"
                rel="noreferrer"
                onClick={(event) => {
                  event.preventDefault();
                  openExternal(forwardURL(forward.localPort));
                }}
                className="text-fg-strong hover:underline"
              >
                127.0.0.1:{forward.localPort}
              </a>
            )}
            {forward.state !== 'failed' && (
              <CopyButton
                what={`${forward.name} forward url`}
                text={forwardURL(forward.localPort)}
              />
            )}
            <span className="text-fg-muted">to {forward.remotePort}</span>
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

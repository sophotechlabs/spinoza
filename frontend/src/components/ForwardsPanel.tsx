import { useState } from 'react';
import { refreshForwards, stopForward, useForwardPolling } from '../lib/portForward';
import { useForwardsStore } from '../store/forwards';

function stateColor(state: string): string {
  if (state === 'failed') {
    return 'text-red-400';
  }
  return 'text-green-400';
}

interface ForwardsPanelProps {
  active?: boolean;
}

export default function ForwardsPanel({ active = true }: ForwardsPanelProps) {
  const forwards = useForwardsStore((state) => state.forwards);
  const [error, setError] = useState<string | null>(null);
  useForwardPolling(active);

  async function stop(id: string) {
    setError(null);
    try {
      await stopForward(id);
      await refreshForwards();
    } catch {
      setError('could not stop the forward');
    }
  }

  if (forwards.length === 0) {
    return (
      <div className="p-3 text-neutral-600">
        No active forwards. Open a Pod or Service and forward a port.
      </div>
    );
  }

  return (
    <div className="p-2">
      {error !== null && <p className="mb-1.5 text-red-400">{error}</p>}
      {forwards.map((forward) => (
        <div key={forward.id} className="flex items-center gap-2 border-b border-neutral-900 py-1">
          <span className={stateColor(forward.state)}>●</span>
          <span className="truncate text-neutral-300">
            {forward.kind.toLowerCase()}/{forward.namespace}/{forward.name}
          </span>
          {forward.state === 'failed' && (
            <span className="truncate text-red-400" title={forward.error}>
              {forward.error}
            </span>
          )}
          {forward.state !== 'failed' && (
            <a
              href={`http://127.0.0.1:${forward.localPort}`}
              target="_blank"
              rel="noreferrer"
              className="text-neutral-100 hover:underline"
            >
              127.0.0.1:{forward.localPort}
            </a>
          )}
          <span className="text-neutral-600">→ {forward.remotePort}</span>
          <button
            type="button"
            onClick={() => void stop(forward.id)}
            className="ml-auto rounded border border-neutral-700 px-1.5 text-neutral-300 hover:bg-neutral-800"
          >
            Stop
          </button>
        </div>
      ))}
    </div>
  );
}

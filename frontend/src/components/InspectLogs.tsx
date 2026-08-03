import { useEffect, useRef, useState } from 'react';
import type { LogRequest } from '../lib/types';
import { useLogEnded, useLogError, useLogLines, useLogOffset } from '../store/logs';
import { useLogStream } from '../lib/useLogStream';
import { scrollToBottom } from '../lib/scroll';

export const INSPECT_LOGS_SUB_ID = 'inspect-logs';

interface InspectLogsProps {
  namespace: string;
  pod: string;
  containers: string[];
  subscribeLogs: (subId: string, request: LogRequest) => void;
  unsubscribeLogs: (subId: string) => void;
}

function followLabel(follow: boolean): string {
  if (follow) {
    return 'Following';
  }
  return 'Follow';
}

export default function InspectLogs({
  namespace,
  pod,
  containers,
  subscribeLogs,
  unsubscribeLogs,
}: InspectLogsProps) {
  const [container, setContainer] = useState(() => containers[0] ?? '');
  const [follow, setFollow] = useState(true);
  const lines = useLogLines(INSPECT_LOGS_SUB_ID);
  const offset = useLogOffset(INSPECT_LOGS_SUB_ID);
  const ended = useLogEnded(INSPECT_LOGS_SUB_ID);
  const error = useLogError(INSPECT_LOGS_SUB_ID);
  const scrollRef = useRef<HTMLDivElement | null>(null);

  const podKey = `${namespace}/${pod}`;
  const [lastPod, setLastPod] = useState(podKey);
  if (podKey !== lastPod) {
    setLastPod(podKey);
    setContainer(containers[0] ?? '');
  }

  useLogStream({
    subId: INSPECT_LOGS_SUB_ID,
    namespace,
    name: pod,
    container,
    subscribeLogs,
    unsubscribeLogs,
  });

  useEffect(() => {
    if (!follow) {
      return;
    }
    scrollToBottom(scrollRef.current);
  }, [lines, follow]);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex shrink-0 items-center gap-2 border-b border-neutral-800 px-3 py-1.5 text-xs">
        {containers.length > 1 && (
          <select
            aria-label="Log container"
            value={container}
            onChange={(event) => {
              setContainer(event.target.value);
            }}
            className="rounded border border-neutral-700 bg-neutral-900 px-1 py-0.5 text-neutral-200"
          >
            {containers.map((name) => (
              <option key={name} value={name}>
                {name}
              </option>
            ))}
          </select>
        )}
        <button
          type="button"
          onClick={() => {
            setFollow((value) => !value);
          }}
          aria-pressed={follow}
          className="rounded border border-neutral-700 px-1.5 py-0.5 text-neutral-300 hover:bg-neutral-800"
        >
          {followLabel(follow)}
        </button>
        {error !== null && <span className="truncate text-red-400">{error}</span>}
        {error === null && ended && <span className="text-neutral-400">stream ended</span>}
      </div>
      <div
        ref={scrollRef}
        className="min-h-0 flex-1 overflow-auto px-3 py-2 font-mono text-[11px] text-neutral-300"
      >
        {lines.length === 0 && <span className="text-neutral-400">Waiting for output…</span>}
        {lines.map((line, index) => (
          <div key={offset + index} className="break-all whitespace-pre-wrap">
            {line}
          </div>
        ))}
      </div>
    </div>
  );
}

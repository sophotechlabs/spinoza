import { useCallback, useEffect, useRef, useState } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import type { LogRequest } from '../lib/types';
import { useLogEnded, useLogError, useLogLines, useLogOffset, useLogRevision } from '../store/logs';
import { useLogStream } from '../lib/useLogStream';
import { cachedSegments, rawSegments } from '../lib/logColor';
import { scrollToBottom } from '../lib/scroll';
import { useLogView } from '../store/settings';

const INSPECT_LOGS_PREFIX = 'inspect-logs';
const LOG_LINE_HEIGHT = 16;

interface InspectLogsProps {
  namespace: string;
  pod: string;
  containers: string[];
  active?: boolean;
  subscribeLogs: (subId: string, request: LogRequest) => void;
  unsubscribeLogs: (subId: string) => void;
}

function followLabel(follow: boolean): string {
  if (follow) {
    return 'Following';
  }
  return 'Follow';
}

function viewLabel(pretty: boolean): string {
  if (pretty) {
    return 'Pretty';
  }
  return 'Raw';
}

export default function InspectLogs({
  namespace,
  pod,
  containers,
  active = true,
  subscribeLogs,
  unsubscribeLogs,
}: InspectLogsProps) {
  const [container, setContainer] = useState(() => containers[0] ?? '');
  const [follow, setFollow] = useState(true);
  const logView = useLogView();
  const [pretty, setPretty] = useState(() => logView === 'pretty');
  const scrollRef = useRef<HTMLDivElement | null>(null);

  const podKey = `${namespace}/${pod}`;
  const [lastPod, setLastPod] = useState(podKey);
  if (podKey !== lastPod) {
    setLastPod(podKey);
    setContainer(containers[0] ?? '');
  }

  const subId = useLogStream({
    prefix: INSPECT_LOGS_PREFIX,
    namespace,
    name: pod,
    container,
    enabled: active,
    subscribeLogs,
    unsubscribeLogs,
  });

  const lines = useLogLines(subId);
  const offset = useLogOffset(subId);
  const revision = useLogRevision(subId);
  const ended = useLogEnded(subId);
  const error = useLogError(subId);

  const virtualizer = useVirtualizer({
    count: lines.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => LOG_LINE_HEIGHT,
    overscan: 40,
  });

  const measure = useCallback(
    (node: HTMLDivElement | null) => {
      virtualizer.measureElement(node);
    },
    [virtualizer],
  );

  useEffect(() => {
    if (!follow) {
      return;
    }
    virtualizer.scrollToIndex(lines.length - 1);
    scrollToBottom(scrollRef.current);
  }, [revision, follow, lines.length, virtualizer]);

  let render = rawSegments;
  if (pretty) {
    render = cachedSegments;
  }

  const items = virtualizer.getVirtualItems();

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex shrink-0 items-center gap-2 border-b border-edge px-3 py-1.5 text-xs">
        {containers.length > 1 && (
          <select
            aria-label="Log container"
            value={container}
            onChange={(event) => {
              setContainer(event.target.value);
            }}
            className="rounded border border-edge-strong bg-surface-raised px-1 py-0.5 text-fg"
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
          className="rounded border border-edge-strong px-1.5 py-0.5 text-fg-soft hover:bg-surface-active"
        >
          {followLabel(follow)}
        </button>
        <button
          type="button"
          onClick={() => {
            setPretty((value) => !value);
          }}
          aria-pressed={pretty}
          title="Format structured log lines"
          className="rounded border border-edge-strong px-1.5 py-0.5 text-fg-soft hover:bg-surface-active"
        >
          {viewLabel(pretty)}
        </button>
        {error !== null && <span className="truncate text-error">{error}</span>}
        {error === null && ended && <span className="text-fg-muted">stream ended</span>}
      </div>
      <div
        ref={scrollRef}
        className="min-h-0 flex-1 overflow-auto px-3 py-2 font-mono text-[11px] text-fg-soft"
      >
        {lines.length === 0 && <span className="text-fg-muted">Waiting for output…</span>}
        <div className="relative w-full" style={{ height: `${virtualizer.getTotalSize()}px` }}>
          {items.map((item) => (
            <div
              key={offset + item.index}
              data-index={item.index}
              ref={measure}
              style={{ transform: `translateY(${item.start}px)` }}
              className="absolute top-0 left-0 w-full break-all whitespace-pre-wrap"
            >
              {render(lines[item.index]).map((segment, part) => (
                <span key={part} className={segment.className}>
                  {segment.text}
                </span>
              ))}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

import { useCallback, useEffect, useRef, useState } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import type { LogRequest } from '../lib/types';
import {
  useLogEnded,
  useLogError,
  useLogLines,
  useLogOffset,
  useLogPods,
  useLogResumed,
  useLogRevision,
  useLogSources,
  useLogsStore,
} from '../store/logs';
import { useLogStream } from '../lib/useLogStream';
import { cachedSegments, rawSegments } from '../lib/logColor';
import { scrollToBottom } from '../lib/scroll';
import { copyText } from '../lib/clipboard';
import { useLogView } from '../store/settings';

const INSPECT_LOGS_PREFIX = 'inspect-logs';
const LOG_LINE_HEIGHT = 16;

interface VisibleLine {
  index: number;
  text: string;
  source: string;
}

function matching(lines: string[], sources: string[], query: string): VisibleLine[] {
  const needle = query.trim().toLowerCase();
  const out: VisibleLine[] = [];
  lines.forEach((text, index) => {
    const source = sources[index] ?? '';
    if (needle !== '' && !`${source} ${text}`.toLowerCase().includes(needle)) {
      return;
    }
    out.push({ index, text, source });
  });
  return out;
}

function wrapClass(wrap: boolean): string {
  if (wrap) {
    return 'break-all whitespace-pre-wrap';
  }
  return 'whitespace-pre';
}

function toggleClass(on: boolean): string {
  const base = 'rounded border px-1.5 py-0.5';
  if (on) {
    return `${base} border-edge-emphasis bg-surface-active text-fg-strong`;
  }
  return `${base} border-edge-strong text-fg-soft hover:bg-surface-active`;
}

function saveAs(name: string, text: string): void {
  const url = URL.createObjectURL(new Blob([text], { type: 'text/plain' }));
  const link = document.createElement('a');
  link.href = url;
  link.download = name;
  link.click();
  URL.revokeObjectURL(url);
}

interface InspectLogsProps {
  namespace: string;
  pod: string;
  containers: string[];
  // Set when the selection is a workload rather than a pod, which tails every
  // pod behind it at once.
  workload?: { group: string; version: string; resource: string };
  active?: boolean;
  subscribeLogs: (subId: string, request: LogRequest) => void;
  unsubscribeLogs: (subId: string) => void;
}

function fileName(namespace: string, pod: string, container: string): string {
  if (container === '') {
    return `${namespace}-${pod}.log`;
  }
  return `${namespace}-${pod}-${container}.log`;
}

function withSource(line: VisibleLine): string {
  if (line.source === '') {
    return line.text;
  }
  return `${line.source} ${line.text}`;
}

function podsLabel(attached: number, matched: number): string {
  if (matched > attached) {
    return `${String(attached)} of ${String(matched)} pods`;
  }
  return `${String(attached)} pods`;
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
  workload,
  active = true,
  subscribeLogs,
  unsubscribeLogs,
}: InspectLogsProps) {
  const [container, setContainer] = useState(() => containers[0] ?? '');
  const [follow, setFollow] = useState(true);
  const logView = useLogView();
  const [pretty, setPretty] = useState(() => logView === 'pretty');
  const [query, setQuery] = useState('');
  const [wrap, setWrap] = useState(true);
  const [withTime, setWithTime] = useState(true);
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const clearLines = useLogsStore((state) => state.clearLines);

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
    workload,
    enabled: active,
    subscribeLogs,
    unsubscribeLogs,
  });

  const lines = useLogLines(subId);
  const sources = useLogSources(subId);
  const pods = useLogPods(subId);
  const offset = useLogOffset(subId);
  const revision = useLogRevision(subId);
  const ended = useLogEnded(subId);
  const error = useLogError(subId);
  const resumed = useLogResumed(subId);

  const visible = matching(lines, sources, query);

  const virtualizer = useVirtualizer({
    count: visible.length,
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
    virtualizer.scrollToIndex(visible.length - 1);
    scrollToBottom(scrollRef.current);
  }, [revision, follow, visible.length, virtualizer]);

  function jumpToBottom() {
    virtualizer.scrollToIndex(visible.length - 1);
    scrollToBottom(scrollRef.current);
  }

  function segmentsFor(text: string) {
    if (pretty) {
      return cachedSegments(text, withTime);
    }
    return rawSegments(text);
  }

  function asText(): string {
    return visible.map(withSource).join('\n');
  }

  function handleDownload() {
    saveAs(fileName(namespace, pod, container), asText());
  }

  function handleCopy() {
    void copyText('log lines', asText());
  }

  function handleClear() {
    clearLines(subId);
  }

  const items = virtualizer.getVirtualItems();

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-edge px-3 py-1.5 text-xs">
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
        <button
          type="button"
          onClick={() => {
            setWithTime((value) => !value);
          }}
          aria-pressed={withTime}
          title="Show the timestamp each line carries"
          className={toggleClass(withTime)}
        >
          Timestamps
        </button>
        <button
          type="button"
          onClick={() => {
            setWrap((value) => !value);
          }}
          aria-pressed={wrap}
          title="Wrap long lines instead of scrolling sideways"
          className={toggleClass(wrap)}
        >
          Wrap
        </button>
        <input
          type="search"
          aria-label="Filter log lines"
          placeholder="Filter"
          value={query}
          onChange={(event) => {
            setQuery(event.target.value);
          }}
          className="w-40 rounded border border-edge bg-surface-raised px-2 py-0.5 text-fg placeholder:text-fg-muted focus:border-edge-emphasis"
        />
        <button
          type="button"
          onClick={handleCopy}
          className="rounded border border-edge-strong px-1.5 py-0.5 text-fg-soft hover:bg-surface-active"
        >
          Copy
        </button>
        <button
          type="button"
          onClick={handleDownload}
          className="rounded border border-edge-strong px-1.5 py-0.5 text-fg-soft hover:bg-surface-active"
        >
          Download
        </button>
        <button
          type="button"
          onClick={handleClear}
          className="rounded border border-edge-strong px-1.5 py-0.5 text-fg-soft hover:bg-surface-active"
        >
          Clear
        </button>
        {!follow && (
          <button
            type="button"
            onClick={jumpToBottom}
            className="rounded border border-edge-strong px-1.5 py-0.5 text-fg-soft hover:bg-surface-active"
          >
            Jump to bottom
          </button>
        )}
        {pods.attached > 1 && (
          <span className="shrink-0 text-fg-muted">{podsLabel(pods.attached, pods.matched)}</span>
        )}
        {query !== '' && (
          <span className="shrink-0 text-fg-muted">
            {visible.length} of {lines.length}
          </span>
        )}
        {error !== null && (
          <span role="alert" className="truncate text-error">
            {error}
          </span>
        )}
        {error === null && ended && (
          <span role="status" className="text-fg-muted">
            stream ended
          </span>
        )}
        {resumed && (
          <span role="status" className="text-warn">
            reconnected, output above may repeat
          </span>
        )}
      </div>
      <div
        ref={scrollRef}
        className="min-h-0 flex-1 overflow-auto px-3 py-2 font-mono text-[11px] text-fg-soft"
      >
        {lines.length === 0 && <span className="text-fg-muted">Waiting for output</span>}
        {lines.length > 0 && visible.length === 0 && (
          <span className="text-fg-muted">No line matches that filter.</span>
        )}
        <div className="relative w-full" style={{ height: `${virtualizer.getTotalSize()}px` }}>
          {items.map((item) => (
            <div
              key={offset + visible[item.index].index}
              data-index={item.index}
              ref={measure}
              style={{ transform: `translateY(${item.start}px)` }}
              className={`absolute top-0 left-0 w-full ${wrapClass(wrap)}`}
            >
              {visible[item.index].source !== '' && (
                <span className="text-fg-muted">{visible[item.index].source} </span>
              )}
              {segmentsFor(visible[item.index].text).map((segment, part) => (
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

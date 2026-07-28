import { useEffect, useRef, useState } from 'react';
import type { LogRequest } from '../lib/types';
import { useLogEnded, useLogLines } from '../store/logs';
import ForwardsPanel from './ForwardsPanel';

export const LOGS_SUB_ID = 'logs';
const TAIL_LINES = 500;

export interface PodTarget {
  namespace: string;
  name: string;
  containers: string[];
}

interface BottomDockProps {
  pod: PodTarget | null;
  subscribeLogs: (subId: string, request: LogRequest) => void;
  unsubscribeLogs: (subId: string) => void;
}

function chevron(open: boolean): string {
  if (open) {
    return '▾';
  }
  return '▸';
}

function firstContainer(pod: PodTarget | null): string {
  if (pod === null) {
    return '';
  }
  if (pod.containers.length === 0) {
    return '';
  }
  return pod.containers[0];
}

function followLabel(follow: boolean): string {
  if (follow) {
    return 'Following';
  }
  return 'Paused';
}

type DockTab = 'logs' | 'forwards';

export default function BottomDock({ pod, subscribeLogs, unsubscribeLogs }: BottomDockProps) {
  const [open, setOpen] = useState(false);
  const [tab, setTab] = useState<DockTab>('logs');
  const [follow, setFollow] = useState(true);
  const [container, setContainer] = useState('');
  const lines = useLogLines(LOGS_SUB_ID);
  const ended = useLogEnded(LOGS_SUB_ID);
  const scrollRef = useRef<HTMLDivElement | null>(null);

  const podKey = pod === null ? '' : `${pod.namespace}/${pod.name}`;
  const podNamespace = pod === null ? '' : pod.namespace;
  const podName = pod === null ? '' : pod.name;

  useEffect(() => {
    setContainer(firstContainer(pod));
  }, [podKey, pod]);

  useEffect(() => {
    if (!open) {
      return;
    }
    if (tab !== 'logs') {
      return;
    }
    if (podName === '') {
      return;
    }
    if (container === '') {
      return;
    }
    subscribeLogs(LOGS_SUB_ID, {
      namespace: podNamespace,
      name: podName,
      container,
      tailLines: TAIL_LINES,
      follow,
    });
    return () => {
      unsubscribeLogs(LOGS_SUB_ID);
    };
  }, [open, tab, podNamespace, podName, container, follow, subscribeLogs, unsubscribeLogs]);

  useEffect(() => {
    if (!follow) {
      return;
    }
    const node = scrollRef.current;
    if (node === null) {
      return;
    }
    node.scrollTop = node.scrollHeight;
  }, [lines, follow]);

  function toggle() {
    setOpen((value) => !value);
  }

  function select(next: DockTab) {
    setTab(next);
    setOpen(true);
  }

  function tabClass(name: DockTab): string {
    if (name === tab) {
      return 'px-2 py-1 text-neutral-100';
    }
    return 'px-2 py-1 text-neutral-500 hover:text-neutral-300';
  }

  function toggleFollow() {
    setFollow((value) => !value);
  }

  function handleContainer(event: React.ChangeEvent<HTMLSelectElement>) {
    setContainer(event.target.value);
  }

  return (
    <div className="shrink-0 border-t border-neutral-800 bg-neutral-900 text-xs">
      <div className="flex items-center">
        <button
          type="button"
          aria-label="Toggle panel"
          onClick={toggle}
          className="px-2 py-1.5 text-neutral-300 hover:bg-neutral-800"
        >
          {chevron(open)}
        </button>
        <button
          type="button"
          onClick={() => {
            select('logs');
          }}
          className={tabClass('logs')}
        >
          Logs
        </button>
        <button
          type="button"
          onClick={() => {
            select('forwards');
          }}
          className={tabClass('forwards')}
        >
          Forwards
        </button>
        <button type="button" disabled className="cursor-not-allowed px-2 py-1 text-neutral-700">
          Terminal
        </button>
        {open && tab === 'logs' && (
          <div className="flex flex-1 items-center gap-2 border-l border-neutral-800 pl-2">
            {pod !== null && (
              <span className="truncate text-neutral-500">
                {pod.namespace}/{pod.name}
              </span>
            )}
            {pod !== null && pod.containers.length > 1 && (
              <select
                aria-label="Container"
                value={container}
                onChange={handleContainer}
                className="rounded border border-neutral-700 bg-neutral-900 px-1 py-0.5 text-neutral-200"
              >
                {pod.containers.map((name) => (
                  <option key={name} value={name}>
                    {name}
                  </option>
                ))}
              </select>
            )}
            <button
              type="button"
              onClick={toggleFollow}
              aria-pressed={follow}
              className="rounded border border-neutral-700 px-1.5 py-0.5 text-neutral-300 hover:bg-neutral-800"
            >
              {followLabel(follow)}
            </button>
            {ended && <span className="text-neutral-600">stream ended</span>}
          </div>
        )}
      </div>
      {open && tab === 'forwards' && (
        <div className="h-56 overflow-auto border-t border-neutral-800 bg-neutral-950 text-[11px]">
          <ForwardsPanel />
        </div>
      )}
      {open && tab === 'logs' && (
        <div
          ref={scrollRef}
          className="h-56 overflow-auto border-t border-neutral-800 bg-neutral-950 p-2 font-mono text-[11px] leading-4 text-neutral-300"
        >
          {pod === null && (
            <span className="text-neutral-600">Select a pod to stream its logs.</span>
          )}
          {pod !== null && lines.length === 0 && (
            <span className="text-neutral-600">Waiting for output…</span>
          )}
          {lines.map((line, index) => (
            <div key={`${index}-${line}`} className="break-all whitespace-pre-wrap">
              {line}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

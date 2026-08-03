import { useEffect, useState } from 'react';
import type { ContainerState, LogRequest, ObjectDetail, ObjectRef } from '../lib/types';
import { containerNames } from '../lib/containers';
import { fetchObject, refQuery } from '../lib/object';
import { NUDGE_STEP, useDrawerWidth } from '../lib/usePanelWidth';
import { isFluxObject } from '../lib/fluxActions';
import { hasActions } from '../lib/objectActions';
import { forwardKind } from '../lib/portForward';
import InspectActions from './InspectActions';
import InspectObjectActions from './InspectObjectActions';
import InspectPorts from './InspectPorts';
import InspectOverview from './InspectOverview';
import InspectYaml from './InspectYaml';
import InspectEvents from './InspectEvents';
import InspectMetrics from './InspectMetrics';
import InspectLogs from './InspectLogs';

type Tab = 'overview' | 'yaml' | 'events' | 'logs' | 'metrics';

const BASE_TABS: { id: Tab; label: string }[] = [
  { id: 'overview', label: 'Overview' },
  { id: 'yaml', label: 'YAML' },
  { id: 'events', label: 'Events' },
];

const METRICS_TAB: { id: Tab; label: string } = { id: 'metrics', label: 'Metrics' };
const LOGS_TAB: { id: Tab; label: string } = { id: 'logs', label: 'Logs' };

function keyOf(target: ObjectRef | null): string {
  if (target === null) {
    return '';
  }
  return refQuery(target);
}

function tabsFor(kind: string | undefined): { id: Tab; label: string }[] {
  if (kind === 'Pod') {
    return [...BASE_TABS, LOGS_TAB, METRICS_TAB];
  }
  return BASE_TABS;
}

interface InspectDrawerProps {
  target: ObjectRef | null;
  containers?: ContainerState[];
  subscribeLogs?: (subId: string, request: LogRequest) => void;
  unsubscribeLogs?: (subId: string) => void;
  onClose: () => void;
  onDeleted: () => void;
}

function logContainers(states: ContainerState[] | undefined, detail: ObjectDetail): string[] {
  if (states !== undefined && states.length > 0) {
    return containerNames(states);
  }
  return detail.containers ?? [];
}

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'object request failed';
}

function forwardable(detail: ObjectDetail): string | null {
  if (detail.ports === undefined) {
    return null;
  }
  if (detail.ports.length === 0) {
    return null;
  }
  return forwardKind(detail.apiVersion, detail.kind);
}

function tabClass(active: boolean): string {
  if (active) {
    return 'border-b-2 border-neutral-300 text-neutral-100';
  }
  return 'border-b-2 border-transparent text-neutral-500 hover:text-neutral-300';
}

export default function InspectDrawer({
  target,
  containers,
  subscribeLogs,
  unsubscribeLogs,
  onClose,
  onDeleted,
}: InspectDrawerProps) {
  const [detail, setDetail] = useState<ObjectDetail | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [tab, setTab] = useState<Tab>('overview');
  const [reload, setReload] = useState(0);
  const [collapsed, setCollapsed] = useState(false);
  const { width, startResize, nudge } = useDrawerWidth();

  const targetKey = keyOf(target);
  const [lastKey, setLastKey] = useState(targetKey);
  if (targetKey !== lastKey) {
    setLastKey(targetKey);
    setDetail(null);
    setError(null);
  }

  useEffect(() => {
    if (target === null) {
      setDetail(null);
      setError(null);
      return;
    }
    let mounted = true;
    const load = async () => {
      try {
        const next = await fetchObject(target);
        if (mounted) {
          setDetail(next);
          setError(null);
        }
      } catch (err: unknown) {
        if (mounted) {
          setDetail(null);
          setError(errorMessage(err));
        }
      }
    };
    void load();
    return () => {
      mounted = false;
    };
  }, [target, reload]);

  if (collapsed) {
    return (
      <aside className="flex w-8 shrink-0 items-start justify-center border-l border-neutral-800 bg-neutral-950 pt-2">
        <button
          type="button"
          aria-label="Show inspector"
          onClick={() => {
            setCollapsed(false);
          }}
          className="rounded px-1 py-0.5 text-xs text-neutral-500 hover:bg-neutral-900 hover:text-neutral-200"
        >
          ‹
        </button>
      </aside>
    );
  }

  if (target === null) {
    return (
      <aside className="flex w-80 shrink-0 items-start gap-2 border-l border-neutral-800 bg-neutral-950 p-4 text-xs text-neutral-500">
        <button
          type="button"
          aria-label="Hide inspector"
          onClick={() => {
            setCollapsed(true);
          }}
          className="rounded px-1 text-neutral-500 hover:bg-neutral-900 hover:text-neutral-200"
        >
          ›
        </button>
        Select a row to inspect it.
      </aside>
    );
  }

  function handleApplied() {
    setReload((value) => value + 1);
  }

  function handleResize(event: React.MouseEvent<HTMLButtonElement>) {
    event.preventDefault();
    startResize(event.clientX);
  }

  function handleResizeKey(event: React.KeyboardEvent<HTMLButtonElement>) {
    if (event.key === 'ArrowLeft') {
      event.preventDefault();
      nudge(NUDGE_STEP);
      return;
    }
    if (event.key === 'ArrowRight') {
      event.preventDefault();
      nudge(-NUDGE_STEP);
    }
  }

  const available = tabsFor(detail?.kind);
  const shown = available.some((entry) => entry.id === tab) ? tab : 'overview';

  let body = <div className="p-4 text-xs text-neutral-600">Loading {target.name}…</div>;
  if (error !== null) {
    body = <div className="p-4 text-xs break-words text-red-400">{error}</div>;
  }
  if (detail !== null && shown === 'overview') {
    body = (
      <div className="overflow-y-auto">
        {forwardable(detail) !== null && detail.ports !== undefined && (
          <InspectPorts target={target} kind={forwardable(detail) ?? ''} ports={detail.ports} />
        )}
        <InspectOverview detail={detail} containers={containers} />
      </div>
    );
  }
  if (detail !== null && shown === 'yaml') {
    body = (
      <InspectYaml
        target={target}
        detail={detail}
        onApplied={handleApplied}
        onDeleted={onDeleted}
      />
    );
  }
  if (detail !== null && shown === 'events') {
    body = <InspectEvents namespace={detail.namespace} uid={detail.uid} />;
  }
  if (
    detail !== null &&
    shown === 'logs' &&
    subscribeLogs !== undefined &&
    unsubscribeLogs !== undefined
  ) {
    body = (
      <InspectLogs
        namespace={detail.namespace}
        pod={detail.name}
        containers={logContainers(containers, detail)}
        subscribeLogs={subscribeLogs}
        unsubscribeLogs={unsubscribeLogs}
      />
    );
  }
  if (detail !== null && shown === 'metrics') {
    body = <InspectMetrics namespace={detail.namespace} pod={detail.name} />;
  }

  return (
    <aside
      style={{ width: `${width}px` }}
      className="flex min-h-0 min-w-0 shrink-0 overflow-hidden border-l border-neutral-800 bg-neutral-950"
    >
      <button
        type="button"
        aria-label="Resize inspector"
        onMouseDown={handleResize}
        onKeyDown={handleResizeKey}
        className="w-1 shrink-0 cursor-col-resize bg-neutral-900 hover:bg-neutral-700"
      />
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">
        <div className="flex items-center gap-2 border-b border-neutral-800 px-3 py-2">
          <span className="shrink-0 text-[11px] text-neutral-500">{target.resource}</span>
          <span className="truncate text-xs font-semibold text-neutral-100">{target.name}</span>
          <button
            type="button"
            onClick={onClose}
            className="ml-auto shrink-0 rounded border border-neutral-700 px-1.5 text-xs text-neutral-300 hover:bg-neutral-800"
          >
            Close
          </button>
          <button
            type="button"
            aria-label="Hide inspector"
            onClick={() => {
              setCollapsed(true);
            }}
            className="shrink-0 rounded border border-neutral-700 px-1.5 text-xs text-neutral-300 hover:bg-neutral-800"
          >
            ›
          </button>
        </div>
        <div className="flex shrink-0 gap-3 border-b border-neutral-800 px-3 text-xs">
          {available.map((entry) => (
            <button
              key={entry.id}
              type="button"
              onClick={() => {
                setTab(entry.id);
              }}
              className={`py-1.5 ${tabClass(shown === entry.id)}`}
            >
              {entry.label}
            </button>
          ))}
        </div>
        {detail !== null && isFluxObject(detail.apiVersion) && (
          <InspectActions target={target} suspended={detail.suspended} onDone={handleApplied} />
        )}
        {detail !== null && hasActions(target) && (
          <InspectObjectActions target={target} detail={detail} onDone={handleApplied} />
        )}
        <div className="flex min-h-0 min-w-0 flex-1 flex-col">{body}</div>
      </div>
    </aside>
  );
}

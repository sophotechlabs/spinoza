import { useEffect, useMemo, useState } from 'react';
import type {
  GitopsApp,
  GitopsIssue,
  GitopsResource,
  Graph,
  GraphNode,
  ObjectRef,
} from '../lib/types';
import { fetchGitopsAppGraph, useGitopsApp } from '../lib/gitopsApp';
import type { ArgoResourceRef } from '../lib/argoActions';
import { runArgoAction } from '../lib/argoActions';
import { confirmName } from '../lib/contexts';
import { useProtectedCluster } from '../store/contexts';
import { healthClass, orDash, syncClass } from '../lib/argoStatus';
import Announce from './Announce';
import ConfirmByName from './ConfirmByName';
import GraphCanvas from './GraphCanvas';
import Loading from './Loading';

const TABS = ['resources', 'activity', 'topology'] as const;

type Tab = (typeof TABS)[number];

const TAB_LABELS: Record<Tab, string> = {
  resources: 'Resources',
  activity: 'Activity',
  topology: 'Topology',
};

const SEVERITY_CLASS: Record<string, string> = {
  fatal: 'border-error-line text-error',
  degraded: 'border-error-line text-error',
  warning: 'border-warn-line text-warn',
  info: 'border-edge-strong text-fg-muted',
};

interface GitopsAppPanelProps {
  target: ObjectRef | null;
  active?: boolean;
  onSelectResource: (ref: ObjectRef) => void;
}

function errorMessage(err: unknown, fallback: string): string {
  if (err instanceof Error) {
    return err.message;
  }
  return fallback;
}

function stoppedReason(stopped: boolean): string | undefined {
  if (stopped) {
    return 'this application is being deleted';
  }
  return undefined;
}

function markedLabel(count: number): string {
  if (count === 1) {
    return 'one marked resource';
  }
  return `${String(count)} marked resources`;
}

function keyOf(one: GitopsResource): string {
  return `${one.group ?? ''}/${one.kind}/${one.namespace ?? ''}/${one.name}`;
}

function markedOf(one: GitopsResource): ArgoResourceRef {
  return { group: one.group, kind: one.kind, name: one.name, namespace: one.namespace };
}

function refOfResource(one: GitopsResource): ObjectRef | null {
  if (one.resource === undefined || one.resource === '' || one.version === undefined) {
    return null;
  }
  return {
    group: one.group ?? '',
    version: one.version,
    resource: one.resource,
    namespace: one.namespace ?? '',
    name: one.name,
  };
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <div className="text-fg-muted">{label}</div>
      <div className="truncate text-fg-strong" title={value}>
        {orDash(value)}
      </div>
    </div>
  );
}

function Issues({ issues }: { issues: GitopsIssue[] }) {
  if (issues.length === 0) {
    return null;
  }
  return (
    <ul className="shrink-0 space-y-1 border-b border-edge px-3 py-2">
      {issues.map((one, at) => (
        <li
          key={`${one.subject ?? ''}-${String(at)}`}
          className={`rounded border px-2 py-1 ${SEVERITY_CLASS[one.severity] ?? SEVERITY_CLASS.info}`}
        >
          <div className="font-semibold">{one.title}</div>
          {one.detail !== undefined && one.detail !== '' && (
            <div className="text-fg-soft">{one.detail}</div>
          )}
        </li>
      ))}
    </ul>
  );
}

function Header({ app }: { app: GitopsApp }) {
  return (
    <div className="shrink-0 border-b border-edge px-3 py-2">
      <div className="grid grid-cols-4 gap-3">
        <Field label="Repository" value={app.source.repo ?? ''} />
        <Field label="Path" value={app.source.path ?? ''} />
        <Field label="Target" value={app.source.target ?? ''} />
        <Field label="Destination" value={app.source.destination ?? ''} />
      </div>
      <div className="mt-2 grid grid-cols-4 gap-3">
        <div className="min-w-0">
          <div className="text-fg-muted">Sync</div>
          <div className={syncClass(app.state.sync ?? '')}>{orDash(app.state.sync ?? '')}</div>
        </div>
        <div className="min-w-0">
          <div className="text-fg-muted">Health</div>
          <div className={healthClass(app.state.health ?? '')}>
            {orDash(app.state.health ?? '')}
          </div>
        </div>
        <Field label="Revision" value={app.state.revision ?? ''} />
        <Field label="Sync mode" value={modeOf(app)} />
      </div>
    </div>
  );
}

function modeOf(app: GitopsApp): string {
  if (app.source.policy === undefined || app.source.policy === '') {
    return app.source.syncMode;
  }
  return `${app.source.syncMode} · ${app.source.policy}`;
}

function Drift({ one }: { one: GitopsResource }) {
  const drift = one.drift ?? [];
  if (drift.length === 0) {
    if (one.driftNote === undefined || one.driftNote === '') {
      return null;
    }
    return <div className="mt-1 text-fg-muted">{one.driftNote}</div>;
  }
  return (
    <div className="mt-1">
      {one.driftOwners === true && one.driftNote !== undefined && (
        <div className="text-fg-muted">{one.driftNote}</div>
      )}
      <ul className="space-y-0.5">
        {drift.map((field) => (
          <li key={field.path} className="font-mono text-fg-soft">
            <span>{field.path}</span> <span className="text-fg-muted">{field.declared}</span> →{' '}
            <span className="text-warn">{field.live}</span>
          </li>
        ))}
        {one.driftOwners !== true && one.driftNote !== undefined && one.driftNote !== '' && (
          <li className="text-fg-muted">{one.driftNote}</li>
        )}
      </ul>
    </div>
  );
}

function Events({ one }: { one: GitopsResource }) {
  const events = one.events ?? [];
  if (events.length === 0) {
    return null;
  }
  return (
    <ul className="mt-1 space-y-0.5">
      {events.map((event, at) => (
        <li key={`${event.reason}-${String(at)}`} className="text-fg-muted">
          <span className={event.type === 'Warning' ? 'text-warn' : 'text-fg-muted'}>
            {event.reason}
          </span>{' '}
          {event.message}
        </li>
      ))}
    </ul>
  );
}

interface ResourcesProps {
  app: GitopsApp;
  marked: Set<string>;
  onMark: (key: string) => void;
  onOpen: (ref: ObjectRef) => void;
}

function Resources({ app, marked, onMark, onOpen }: ResourcesProps) {
  if (app.resources === undefined || app.resources.length === 0) {
    return <p className="p-3 text-fg-muted">This object records no managed resources.</p>;
  }
  return (
    <ul className="divide-y divide-edge">
      {app.resources.map((one) => {
        const key = keyOf(one);
        const ref = refOfResource(one);
        return (
          <li key={key} className="px-3 py-2">
            <div className="flex items-baseline gap-2">
              <input
                type="checkbox"
                aria-label={`Mark ${one.kind} ${one.name}`}
                checked={marked.has(key)}
                onChange={() => {
                  onMark(key);
                }}
              />
              <span className="w-32 shrink-0 truncate text-fg-muted">{one.kind}</span>
              {ref === null && (
                <span className="min-w-0 flex-1 truncate text-fg-strong">{one.name}</span>
              )}
              {ref !== null && (
                <button
                  type="button"
                  onClick={() => {
                    onOpen(ref);
                  }}
                  className="min-w-0 flex-1 truncate text-left text-fg-strong hover:underline"
                >
                  {one.name}
                </button>
              )}
              <span className="w-24 shrink-0 truncate text-fg-muted">
                {orDash(one.namespace ?? '')}
              </span>
              <span className={`w-24 shrink-0 truncate ${syncClass(one.sync ?? '')}`}>
                {orDash(one.sync ?? '')}
              </span>
              <span className={`w-24 shrink-0 truncate ${healthClass(one.health ?? '')}`}>
                {orDash(one.health ?? '')}
              </span>
              {one.terminating === true && <span className="shrink-0 text-warn">Terminating</span>}
            </div>
            {one.terminating === true && (one.finalizers ?? []).length > 0 && (
              <div className="mt-1 text-fg-muted">held by {(one.finalizers ?? []).join(', ')}</div>
            )}
            <Drift one={one} />
            <Events one={one} />
          </li>
        );
      })}
    </ul>
  );
}

interface ActivityProps {
  app: GitopsApp;
  busy: boolean;
  onRollback: (id: number) => void;
}

function Activity({ app, busy, onRollback }: ActivityProps) {
  const history = [...(app.history ?? [])].reverse();
  const rollbackable = app.controller === 'argocd';
  return (
    <div>
      {app.operation !== undefined && (
        <div className="border-b border-edge px-3 py-2">
          <div className="flex items-baseline gap-2">
            <span className="text-fg-strong">{app.operation.phase}</span>
            {app.operation.running === true && <span className="text-fg-muted">in flight</span>}
            <span className="text-fg-muted">{app.operation.initiatedBy ?? ''}</span>
            <span className="ml-auto text-fg-muted">
              {app.operation.finishedAt ?? app.operation.startedAt ?? ''}
            </span>
          </div>
          {app.operation.cause !== undefined && app.operation.cause !== '' && (
            <div className="mt-1 text-warn">{app.operation.cause}</div>
          )}
          {app.operation.message !== undefined && app.operation.message !== '' && (
            <div className="mt-1 text-fg-soft">{app.operation.message}</div>
          )}
        </div>
      )}
      {history.length === 0 && <p className="p-3 text-fg-muted">No deployments recorded yet.</p>}
      <ul className="divide-y divide-edge">
        {history.map((entry) => (
          <li key={entry.id} className="flex items-baseline gap-2 px-3 py-2">
            <span className="w-10 shrink-0 text-fg-muted">{entry.id}</span>
            <span
              className="min-w-0 flex-1 truncate font-mono text-fg-strong"
              title={entry.revision}
            >
              {orDash(entry.revision)}
            </span>
            <span className="w-40 shrink-0 truncate text-fg-muted">
              {entry.automated === true ? 'automation' : orDash(entry.initiatedBy ?? '')}
            </span>
            <span className="w-48 shrink-0 truncate text-fg-muted">
              {orDash(entry.deployedAt ?? '')}
            </span>
            {rollbackable && (
              <button
                type="button"
                disabled={busy}
                onClick={() => {
                  onRollback(entry.id);
                }}
                className="shrink-0 rounded border border-edge-strong px-2 py-0.5 text-fg hover:bg-surface-active disabled:cursor-not-allowed disabled:text-fg-faint"
              >
                Roll back
              </button>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}

function Topology({
  target,
  epoch,
  onSelect,
}: {
  target: ObjectRef;
  epoch: number;
  onSelect: (node: GraphNode) => void;
}) {
  const [graph, setGraph] = useState<Graph | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [reloads, setReloads] = useState(0);

  useEffect(() => {
    let mounted = true;
    fetchGitopsAppGraph(target)
      .then((found) => {
        if (mounted) {
          setGraph(found);
          setError(null);
        }
      })
      .catch((err: unknown) => {
        if (mounted) {
          setError(errorMessage(err, 'the graph request failed'));
        }
      });
    return () => {
      mounted = false;
    };
  }, [target, reloads, epoch]);

  return (
    <GraphCanvas
      what="The managed resources"
      empty="This object manages nothing yet."
      data={graph}
      error={error}
      onRetry={() => {
        setReloads((value) => value + 1);
      }}
      onSelect={onSelect}
    />
  );
}

interface Pending {
  action: 'sync' | 'rollback';
  options: Record<string, unknown>;
  what: string;
}

interface AppViewProps {
  target: ObjectRef;
  app: GitopsApp;
  reload: () => void;
  onSelectResource: (ref: ObjectRef) => void;
}

function AppView({ target, app, reload, onSelectResource }: AppViewProps) {
  const [tab, setTab] = useState<Tab>('resources');
  const [marked, setMarked] = useState<Set<string>>(new Set());
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [asking, setAsking] = useState<Pending | null>(null);
  const [epoch, setEpoch] = useState(0);
  const protectedCluster = useProtectedCluster();

  useEffect(() => {
    setMarked(new Set());
    setNotice(null);
    setActionError(null);
    setAsking(null);
  }, [target]);

  const markedRefs = useMemo<ArgoResourceRef[]>(
    () => (app.resources ?? []).filter((one) => marked.has(keyOf(one))).map(markedOf),
    [app, marked],
  );

  async function run(pending: Pending) {
    setBusy(true);
    setNotice(null);
    setActionError(null);
    setAsking(null);
    try {
      await runArgoAction(
        target,
        pending.action,
        confirmName(protectedCluster, target.name),
        pending.options,
      );
      setNotice(pending.action === 'sync' ? 'Sync requested.' : 'Rollback requested.');
      setMarked(new Set());
      reload();
      setEpoch((value) => value + 1);
    } catch (err: unknown) {
      setActionError(errorMessage(err, 'action failed'));
    } finally {
      setBusy(false);
    }
  }

  function ask(pending: Pending) {
    if (protectedCluster) {
      setAsking(pending);
      return;
    }
    void run(pending);
  }

  function askRollback(id: number) {
    ask({
      action: 'rollback',
      options: { revision: id },
      what: `Rolling ${target.name} back to deployment ${String(id)}.`,
    });
  }

  const syncable = app.controller === 'argocd';
  const stopped = app.terminating === true;

  return (
    <div className="flex h-full min-h-0 flex-col text-xs">
      <Header app={app} />
      <Issues issues={app.issues ?? []} />
      <div className="flex shrink-0 items-center gap-2 border-b border-edge px-3 py-1.5">
        {TABS.map((one) => (
          <button
            key={one}
            type="button"
            onClick={() => {
              setTab(one);
            }}
            className={
              one === tab
                ? 'rounded border border-edge-strong bg-surface-active px-2 py-0.5 text-fg-strong'
                : 'rounded border border-transparent px-2 py-0.5 text-fg-muted hover:bg-surface-raised'
            }
          >
            {TAB_LABELS[one]}
          </button>
        ))}
        {syncable && markedRefs.length > 0 && (
          <button
            type="button"
            disabled={busy || stopped}
            title={stoppedReason(stopped)}
            onClick={() => {
              ask({
                action: 'sync',
                options: { resources: markedRefs },
                what: `Syncing ${markedLabel(markedRefs.length)} of ${target.name}.`,
              });
            }}
            className="ml-auto rounded border border-warn-line px-2 py-0.5 text-warn hover:bg-warn-tint disabled:cursor-not-allowed disabled:text-fg-faint"
          >
            Sync {markedRefs.length} marked
          </button>
        )}
      </div>
      <Announce message={actionError} urgent className="px-3 py-1 break-words text-error" />
      <Announce message={notice} className="px-3 py-1 break-words text-ok" />
      <div className="min-h-0 flex-1 overflow-y-auto">
        {tab === 'resources' && (
          <Resources
            app={app}
            marked={marked}
            onMark={(key) => {
              setMarked((current) => {
                const next = new Set(current);
                if (next.has(key)) {
                  next.delete(key);
                } else {
                  next.add(key);
                }
                return next;
              });
            }}
            onOpen={onSelectResource}
          />
        )}
        {tab === 'activity' && (
          <Activity app={app} busy={busy || stopped} onRollback={askRollback} />
        )}
        {tab === 'topology' && (
          <Topology
            target={target}
            epoch={epoch}
            onSelect={(node) => {
              onSelectResource({
                group: node.group,
                version: node.version,
                resource: node.resource,
                namespace: node.namespace,
                name: node.name,
              });
            }}
          />
        )}
      </div>
      {asking !== null && (
        <ConfirmByName
          open
          name={target.name}
          what={asking.what}
          onConfirm={() => void run(asking)}
          onCancel={() => {
            setAsking(null);
          }}
        />
      )}
    </div>
  );
}

export default function GitopsAppPanel({
  target,
  active = true,
  onSelectResource,
}: GitopsAppPanelProps) {
  const { data, error, reload } = useGitopsApp(target, active);

  if (target === null) {
    return (
      <p className="p-4 text-xs text-fg-muted">Select an Argo application or a Flux applier.</p>
    );
  }
  if (error !== null && data === null) {
    return <p className="p-4 text-xs text-error">{error}</p>;
  }
  if (data === null) {
    return <Loading what="the application" />;
  }
  return <AppView target={target} app={data} reload={reload} onSelectResource={onSelectResource} />;
}

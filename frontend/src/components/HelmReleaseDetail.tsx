import { useEffect, useRef, useState } from 'react';
import type {
  HelmReleaseDetail as HelmReleaseDetailData,
  HelmResource,
  HelmRevision,
  ObjectRef,
} from '../lib/types';
import {
  fetchHelmHistory,
  fetchHelmRelease,
  refOf,
  rollbackRelease,
  statusText,
  uninstallRelease,
  useHelmRelease,
  useHelmSupport,
} from '../lib/helm';
import { ago } from '../lib/time';
import { useNow } from '../lib/useNow';
import { useHelmAccess } from '../lib/useHelmAccess';
import { useHelmRefusal } from '../store/helmAccess';
import { notifyError, notifyOk } from '../store/toasts';
import { useContextScope, useProtectedCluster } from '../store/contexts';
import { confirmName } from '../lib/contexts';
import { bumpHelmEpoch } from '../store/helm';
import Announce from './Announce';
import ConfirmByName from './ConfirmByName';
import CopyButton from './CopyButton';
import HelmUpgradeDialog from './HelmUpgradeDialog';
import Loading from './Loading';

const TABS = ['Overview', 'Values', 'Notes', 'Manifest', 'Resources', 'History'] as const;

type Tab = (typeof TABS)[number];

interface HelmReleaseDetailProps {
  namespace: string;
  name: string;
  onSelectResource: (ref: ObjectRef) => void;
  onOpenResource: (ref: ObjectRef, kind: string) => void;
  onClose: () => void;
}

function tabClass(active: boolean): string {
  const base = 'rounded px-2 py-0.5';
  if (active) {
    return `${base} bg-surface-active text-fg-strong`;
  }
  return `${base} text-fg-soft hover:bg-surface-raised`;
}

function Pane({ body, empty }: { body: string; empty: string }) {
  if (body === '') {
    return <p className="p-3 text-fg-muted">{empty}</p>;
  }
  return (
    <pre className="overflow-auto p-3 font-mono text-[11px] whitespace-pre-wrap text-fg-soft">
      {body}
    </pre>
  );
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex gap-2">
      <span className="w-28 shrink-0 text-fg-muted">{label}</span>
      <span className="min-w-0 break-words text-fg-soft">{value}</span>
    </div>
  );
}

interface TypedConfirm {
  what: 'rollback' | 'uninstall';
  revision: number;
  question: string;
}

function reasonFor(
  what: string,
  helmReady: boolean,
  helmReason: string,
  refused: string | null,
): string {
  if (!helmReady) {
    return helmReason;
  }
  if (refused !== null) {
    return refused;
  }
  return what;
}

function orDash(value: string): string {
  if (value === '') {
    return '-';
  }
  return value;
}

export default function HelmReleaseDetail({
  namespace,
  name,
  onSelectResource,
  onOpenResource,
  onClose,
}: HelmReleaseDetailProps) {
  const { data, error, loading, reload } = useHelmRelease(namespace, name);
  const support = useHelmSupport();
  const [tab, setTab] = useState<Tab>('Overview');
  const [busy, setBusy] = useState(false);
  const [confirming, setConfirming] = useState<'uninstall' | null>(null);
  const [typed, setTyped] = useState<TypedConfirm | null>(null);
  const [upgrading, setUpgrading] = useState(false);
  const protectedCluster = useProtectedCluster();
  const [failure, setFailure] = useState<string | null>(null);
  const [history, setHistory] = useState<HelmRevision[]>([]);
  const [historyNext, setHistoryNext] = useState<number | null>(null);
  const [historyError, setHistoryError] = useState<string | null>(null);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyKey, setHistoryKey] = useState('');
  const [inspected, setInspected] = useState<HelmReleaseDetailData | null>(null);
  const [inspecting, setInspecting] = useState<number | null>(null);
  const now = useNow();
  const clusterScope = useContextScope();
  const actionScope = `${clusterScope}|${namespace}/${name}`;
  const liveAction = useRef(actionScope);
  liveAction.current = actionScope;
  const operation = useRef(0);
  const historyOperation = useRef(0);
  const inspectOperation = useRef(0);
  const [stateScope, setStateScope] = useState(actionScope);

  useHelmAccess(namespace, name);
  const noUpgrade = useHelmRefusal(namespace, name, 'upgrade');
  const noRollback = useHelmRefusal(namespace, name, 'rollback');
  const noUninstall = useHelmRefusal(namespace, name, 'uninstall');

  const helmReady = support?.available === true;
  const helmReason = support?.reason ?? 'checking whether helm is available';
  const fluxRef = data?.release.fluxRef;
  const shown = inspected ?? data;

  if (stateScope !== actionScope) {
    setStateScope(actionScope);
    setBusy(false);
    setConfirming(null);
    setTyped(null);
    setUpgrading(false);
    setFailure(null);
    setHistory([]);
    setHistoryNext(null);
    setHistoryError(null);
    setHistoryLoading(false);
    setHistoryKey('');
    setInspected(null);
    setInspecting(null);
  }

  useEffect(() => {
    const scope = actionScope;
    return () => {
      if (liveAction.current === scope) {
        liveAction.current = '';
      }
    };
  }, [actionScope]);

  useEffect(() => {
    if (tab !== 'History' || data === null) {
      return;
    }
    const key = `${actionScope}|${String(data.release.revision)}`;
    if (historyKey === key) {
      return;
    }
    const scope = actionScope;
    historyOperation.current += 1;
    const token = historyOperation.current;
    setHistoryKey(key);
    setHistory([]);
    setHistoryNext(null);
    setHistoryError(null);
    setHistoryLoading(true);
    fetchHelmHistory(namespace, name, data.release.revision)
      .then((page) => {
        if (liveAction.current !== scope || historyOperation.current !== token) {
          return;
        }
        setHistory(page.revisions);
        setHistoryNext(page.next ?? null);
      })
      .catch((err: unknown) => {
        if (liveAction.current !== scope || historyOperation.current !== token) {
          return;
        }
        if (err instanceof Error) {
          setHistoryError(err.message);
        } else {
          setHistoryError('the release history could not be loaded');
        }
      })
      .finally(() => {
        if (liveAction.current === scope && historyOperation.current === token) {
          setHistoryLoading(false);
        }
      });
  }, [actionScope, data, historyKey, name, namespace, tab]);

  async function loadOlderHistory(through: number) {
    const scope = actionScope;
    historyOperation.current += 1;
    const token = historyOperation.current;
    setHistoryLoading(true);
    setHistoryError(null);
    try {
      const page = await fetchHelmHistory(namespace, name, through);
      if (liveAction.current !== scope || historyOperation.current !== token) {
        return;
      }
      setHistory((loaded) => {
        const revisions = new Map(loaded.map((entry) => [entry.revision, entry]));
        for (const entry of page.revisions) {
          revisions.set(entry.revision, entry);
        }
        return [...revisions.values()].sort((left, right) => right.revision - left.revision);
      });
      setHistoryNext(page.next ?? null);
    } catch (err: unknown) {
      if (liveAction.current !== scope || historyOperation.current !== token) {
        return;
      }
      if (err instanceof Error) {
        setHistoryError(err.message);
      } else {
        setHistoryError('the older release history could not be loaded');
      }
    } finally {
      if (liveAction.current === scope && historyOperation.current === token) {
        setHistoryLoading(false);
      }
    }
  }

  async function inspectRevision(revision: number) {
    const scope = actionScope;
    inspectOperation.current += 1;
    const token = inspectOperation.current;
    setInspecting(revision);
    setFailure(null);
    try {
      const detail = await fetchHelmRelease(namespace, name, revision);
      if (liveAction.current !== scope || inspectOperation.current !== token) {
        return;
      }
      setInspected(detail);
      setTab('Overview');
    } catch (err: unknown) {
      if (liveAction.current !== scope || inspectOperation.current !== token) {
        return;
      }
      if (err instanceof Error) {
        setFailure(err.message);
      } else {
        setFailure('the selected revision could not be loaded');
      }
    } finally {
      if (liveAction.current === scope && inspectOperation.current === token) {
        setInspecting(null);
      }
    }
  }

  async function act(what: 'rollback' | 'uninstall', revision: number) {
    const scope = actionScope;
    operation.current += 1;
    const token = operation.current;
    setBusy(true);
    setFailure(null);
    setTyped(null);
    const confirm = confirmName(protectedCluster, name);
    try {
      let work: ReturnType<typeof uninstallRelease>;
      if (what === 'uninstall') {
        work = uninstallRelease(namespace, name, confirm);
      } else {
        work = rollbackRelease(namespace, name, revision, confirm);
      }
      const result = await work;
      if (liveAction.current !== scope || operation.current !== token) {
        return;
      }
      notifyOk(result.message);
      bumpHelmEpoch();
      if (what === 'uninstall') {
        onClose();
        return;
      }
      setInspected(null);
      setHistoryKey('');
      reload();
    } catch (err: unknown) {
      if (liveAction.current !== scope || operation.current !== token) {
        return;
      }
      const message = err instanceof Error ? err.message : 'the release action failed';
      setFailure(message);
      notifyError(message);
    } finally {
      if (liveAction.current === scope && operation.current === token) {
        setBusy(false);
        setConfirming(null);
      }
    }
  }

  function askUninstall() {
    if (protectedCluster) {
      setTyped({
        what: 'uninstall',
        revision: 0,
        question: `Uninstalling ${name}. This cannot be undone.`,
      });
      return;
    }
    setConfirming('uninstall');
  }

  function askRollback(revision: number) {
    if (protectedCluster) {
      setTyped({
        what: 'rollback',
        revision,
        question: `Rolling ${name} back to revision ${String(revision)}.`,
      });
      return;
    }
    void act('rollback', revision);
  }

  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      {typed !== null && (
        <ConfirmByName
          open
          name={name}
          what={typed.question}
          onConfirm={() => void act(typed.what, typed.revision)}
          onCancel={() => {
            setTyped(null);
          }}
        />
      )}
      {upgrading && data !== null && (
        <HelmUpgradeDialog
          release={data.release}
          currentValues={data.values}
          currentManifest={data.manifest}
          onClose={() => {
            setUpgrading(false);
          }}
          onUpgraded={() => {
            bumpHelmEpoch();
            setInspected(null);
            setHistoryKey('');
            reload();
          }}
        />
      )}
      <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-edge px-3 py-1.5">
        <span className="font-semibold text-fg-strong">{name}</span>
        <span className="text-fg-muted">{namespace}</span>
        {TABS.map((name) => (
          <button
            key={name}
            type="button"
            aria-pressed={tab === name}
            onClick={() => {
              setTab(name);
            }}
            className={tabClass(tab === name)}
          >
            {name}
          </button>
        ))}
        <div className="ml-auto flex items-center gap-2">
          {fluxRef !== undefined && (
            <button
              type="button"
              title="Flux manages this release. A helm upgrade here goes back at the next reconcile."
              onClick={() => {
                onSelectResource(fluxRef);
              }}
              className="rounded border border-edge-strong px-1.5 py-0.5 text-fg-soft hover:bg-surface-active"
            >
              Managed by Flux
            </button>
          )}
          {fluxRef === undefined && confirming === null && (
            <button
              type="button"
              disabled={busy || !helmReady || data === null || noUpgrade !== null}
              title={reasonFor('Upgrade this release', helmReady, helmReason, noUpgrade)}
              onClick={() => {
                setUpgrading(true);
              }}
              className="rounded border border-edge-strong px-1.5 py-0.5 text-fg-soft hover:bg-surface-active disabled:cursor-not-allowed disabled:border-edge disabled:text-fg-faint"
            >
              Upgrade
            </button>
          )}
          {confirming === null && (
            <button
              type="button"
              disabled={busy || !helmReady || noUninstall !== null}
              title={reasonFor('Uninstall this release', helmReady, helmReason, noUninstall)}
              onClick={() => {
                askUninstall();
              }}
              className="rounded border border-error-line-strong px-1.5 py-0.5 text-error hover:bg-error-tint disabled:cursor-not-allowed disabled:border-edge disabled:text-fg-faint"
            >
              Uninstall
            </button>
          )}
          {confirming === 'uninstall' && (
            <>
              <span className="text-error">Uninstall {name}? This cannot be undone.</span>
              <button
                type="button"
                disabled={busy}
                onClick={() => void act('uninstall', 0)}
                className="rounded border border-error-line-strong px-1.5 py-0.5 text-error hover:bg-error-tint"
              >
                Confirm
              </button>
              <button
                type="button"
                onClick={() => {
                  setConfirming(null);
                }}
                className="rounded border border-edge-strong px-1.5 py-0.5 text-fg-soft hover:bg-surface-active"
              >
                Cancel
              </button>
            </>
          )}
          <button
            type="button"
            aria-label="Close the release detail"
            onClick={onClose}
            className="rounded border border-edge-strong px-1.5 py-0.5 text-fg-soft hover:bg-surface-active"
          >
            ✕
          </button>
        </div>
      </div>

      <Announce message={failure} urgent className="px-3 py-1 text-error" />
      {!helmReady && support !== null && (
        <p role="status" className="px-3 py-1 text-warn">
          Upgrade, rollback and uninstall need the helm binary: {helmReason}
        </p>
      )}

      {loading && data === null && <Loading what="the release" />}
      {error !== null && (
        <p role="alert" className="p-3 text-error">
          {error}
        </p>
      )}
      {shown?.error !== undefined && (
        <p role="status" className="px-3 py-1 text-warn">
          {shown.error}
        </p>
      )}

      {inspected !== null && data !== null && (
        <div className="flex items-center gap-2 border-b border-edge px-3 py-1 text-warn">
          <span>Viewing stored revision {inspected.release.revision}.</span>
          <button
            type="button"
            onClick={() => {
              setInspected(null);
            }}
            className="rounded border border-edge-strong px-1.5 py-0.5 text-fg-soft hover:bg-surface-active"
          >
            Back to current revision {data.release.revision}
          </button>
        </div>
      )}

      {shown !== null && (
        <div className="min-h-0 flex-1 overflow-auto">
          {tab === 'Overview' && (
            <div className="space-y-1 p-3">
              {shown.release.fluxRef !== undefined && (
                <Field
                  label="Managed by"
                  value={`Flux ${shown.release.fluxRef.namespace}/${shown.release.fluxRef.name}`}
                />
              )}
              <Field label="Chart" value={orDash(shown.release.chart)} />
              <Field label="Chart version" value={orDash(shown.release.chartVersion)} />
              <Field label="App version" value={orDash(shown.release.appVersion)} />
              <Field label="Latest" value={orDash(shown.release.latest ?? '')} />
              <Field label="Revision" value={String(shown.release.revision)} />
              <Field label="Status" value={orDash(shown.release.status)} />
              <Field label="Description" value={orDash(shown.release.description ?? '')} />
              <Field label="First deployed" value={orDash(shown.firstDeployed ?? '')} />
              <Field label="Last deployed" value={orDash(shown.release.updated)} />
              <Field label="Stored in" value={`${shown.driver}s`} />
            </div>
          )}
          {tab === 'Values' && (
            <div>
              <div className="flex justify-end px-3 pt-2">
                <CopyButton what="release values" text={shown.values} />
              </div>
              <Pane
                body={shown.values}
                empty="This release was installed with the chart defaults."
              />
            </div>
          )}
          {tab === 'Notes' && <Pane body={shown.notes} empty="This chart renders no notes." />}
          {tab === 'Manifest' && (
            <div>
              <div className="flex justify-end px-3 pt-2">
                <CopyButton what="release manifest" text={shown.manifest} />
              </div>
              <Pane body={shown.manifest} empty="This release rendered no manifest." />
            </div>
          )}
          {tab === 'Resources' && <Resources resources={shown.resources} onOpen={onOpenResource} />}
          {tab === 'History' && (
            <History
              revisions={history}
              current={data?.release.revision ?? shown.release.revision}
              now={now}
              busy={busy}
              loading={historyLoading}
              error={historyError}
              next={historyNext}
              inspecting={inspecting}
              helmReady={helmReady}
              helmReason={helmReason}
              refused={noRollback}
              onInspect={(revision) => {
                void inspectRevision(revision);
              }}
              onLoadOlder={(through) => {
                void loadOlderHistory(through);
              }}
              onRetry={() => {
                setHistoryKey('');
              }}
              onRollback={(revision) => {
                askRollback(revision);
              }}
            />
          )}
        </div>
      )}
    </div>
  );
}

function Resources({
  resources,
  onOpen,
}: {
  resources: HelmResource[];
  onOpen: (ref: ObjectRef, kind: string) => void;
}) {
  if (resources.length === 0) {
    return <p className="p-3 text-fg-muted">This release rendered no resources.</p>;
  }
  return (
    <table className="w-full border-collapse text-left">
      <thead className="text-fg-muted">
        <tr className="border-b border-edge">
          <th className="px-3 py-1 font-medium">Kind</th>
          <th className="px-3 py-1 font-medium">Name</th>
          <th className="px-3 py-1 font-medium">Namespace</th>
        </tr>
      </thead>
      <tbody>
        {resources.map((resource) => (
          <tr
            key={`${resource.apiVersion}/${resource.kind}/${resource.namespace ?? ''}/${resource.name}`}
            className="border-b border-edge hover:bg-surface-raised"
          >
            <td className="px-3 py-1 text-fg-muted">{resource.kind}</td>
            <td className="px-3 py-1 text-fg-strong">
              <ResourceName resource={resource} onOpen={onOpen} />
            </td>
            <td className="px-3 py-1 text-fg-muted">{orDash(resource.namespace ?? '')}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function ResourceName({
  resource,
  onOpen,
}: {
  resource: HelmResource;
  onOpen: (ref: ObjectRef, kind: string) => void;
}) {
  const ref = refOf(resource);
  if (ref === null) {
    return <span title="this cluster does not report that kind">{resource.name}</span>;
  }
  return (
    <button
      type="button"
      title={`Open ${resource.name} in its table`}
      onClick={() => {
        onOpen(ref, resource.kind);
      }}
      className="hover:underline"
    >
      {resource.name}
    </button>
  );
}

function History({
  revisions,
  current,
  now,
  busy,
  loading,
  error,
  next,
  inspecting,
  helmReady,
  helmReason,
  refused,
  onInspect,
  onLoadOlder,
  onRetry,
  onRollback,
}: {
  revisions: HelmRevision[];
  current: number;
  now: number;
  busy: boolean;
  loading: boolean;
  error: string | null;
  next: number | null;
  inspecting: number | null;
  helmReady: boolean;
  helmReason: string;
  refused: string | null;
  onInspect: (revision: number) => void;
  onLoadOlder: (through: number) => void;
  onRetry: () => void;
  onRollback: (revision: number) => void;
}) {
  if (loading && revisions.length === 0) {
    return <p className="p-3 text-fg-muted">Loading release history…</p>;
  }
  if (error !== null && revisions.length === 0) {
    return (
      <div className="flex items-center gap-2 p-3 text-error">
        <p role="alert">{error}</p>
        <button
          type="button"
          onClick={onRetry}
          className="rounded border border-edge-strong px-1.5 py-0.5 text-fg-soft hover:bg-surface-active"
        >
          Retry history
        </button>
      </div>
    );
  }
  if (revisions.length === 0) {
    return <p className="p-3 text-fg-muted">This release has no stored revisions.</p>;
  }
  return (
    <div>
      <table className="w-full border-collapse text-left">
        <thead className="text-fg-muted">
          <tr className="border-b border-edge">
            <th className="px-3 py-1 text-right font-medium">Rev</th>
            <th className="px-3 py-1 font-medium">Status</th>
            <th className="px-3 py-1 font-medium">Chart</th>
            <th className="px-3 py-1 font-medium">Description</th>
            <th className="px-3 py-1 text-right font-medium">Updated</th>
            <th className="px-3 py-1 text-right font-medium" />
          </tr>
        </thead>
        <tbody>
          {revisions.map((entry) => (
            <tr key={entry.revision} className="border-b border-edge hover:bg-surface-raised">
              <td className="px-3 py-1 text-right text-fg-strong">{entry.revision}</td>
              <td className={`px-3 py-1 ${statusText(entry.status)}`}>{entry.status}</td>
              <td className="px-3 py-1 text-fg-muted">{orDash(entry.chartVersion)}</td>
              <td className="px-3 py-1 text-fg-muted">{orDash(entry.description ?? '')}</td>
              <td className="px-3 py-1 text-right text-fg-muted" title={entry.updated}>
                {ago(entry.updated, now)}
              </td>
              <td className="flex justify-end gap-1 px-3 py-1">
                {entry.revision === current && <span className="text-fg-muted">Current</span>}
                {entry.revision !== current && (
                  <>
                    <button
                      type="button"
                      disabled={inspecting !== null}
                      onClick={() => {
                        onInspect(entry.revision);
                      }}
                      className="rounded border border-edge-strong px-1.5 py-0.5 text-fg-soft hover:bg-surface-active disabled:cursor-not-allowed disabled:text-fg-faint"
                    >
                      {inspecting === entry.revision ? 'Loading…' : 'Inspect'}
                    </button>
                    <button
                      type="button"
                      disabled={busy || !helmReady || refused !== null}
                      title={reasonFor(
                        `Roll back to revision ${String(entry.revision)}`,
                        helmReady,
                        helmReason,
                        refused,
                      )}
                      onClick={() => {
                        onRollback(entry.revision);
                      }}
                      className="rounded border border-edge-strong px-1.5 py-0.5 text-fg-soft hover:bg-surface-active disabled:cursor-not-allowed disabled:text-fg-faint"
                    >
                      Roll back
                    </button>
                  </>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <div className="flex items-center gap-2 p-3">
        {error !== null && <p className="text-error">{error}</p>}
        {next !== null && (
          <button
            type="button"
            disabled={loading}
            onClick={() => {
              onLoadOlder(next);
            }}
            className="rounded border border-edge-strong px-1.5 py-0.5 text-fg-soft hover:bg-surface-active disabled:cursor-not-allowed disabled:text-fg-faint"
          >
            {loading ? 'Loading…' : 'Load older revisions'}
          </button>
        )}
      </div>
    </div>
  );
}

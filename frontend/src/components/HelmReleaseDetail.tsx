import { useState } from 'react';
import type { HelmResource, ObjectRef } from '../lib/types';
import {
  refOf,
  rollbackRelease,
  statusText,
  uninstallRelease,
  useHelmRelease,
  useHelmSupport,
} from '../lib/helm';
import { ago } from '../lib/time';
import { useNow } from '../lib/useNow';
import { notifyError, notifyOk } from '../store/toasts';
import { useProtectedCluster } from '../store/contexts';
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
  const now = useNow();

  const helmReady = support?.available === true;
  const helmReason = support?.reason ?? 'checking whether helm is available';
  const fluxRef = data?.release.fluxRef;

  async function act(what: 'rollback' | 'uninstall', revision: number) {
    setBusy(true);
    setFailure(null);
    setTyped(null);
    const confirm = confirmName(protectedCluster, name);
    try {
      const result =
        what === 'uninstall'
          ? await uninstallRelease(namespace, name, confirm)
          : await rollbackRelease(namespace, name, revision, confirm);
      notifyOk(result.message);
      bumpHelmEpoch();
      if (what === 'uninstall') {
        onClose();
        return;
      }
      reload();
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : 'the release action failed';
      setFailure(message);
      notifyError(message);
    } finally {
      setBusy(false);
      setConfirming(null);
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
              title="Flux manages this release, change it there"
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
              disabled={busy || !helmReady || data === null}
              title={helmReady ? 'Upgrade this release' : helmReason}
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
              disabled={busy || !helmReady}
              title={helmReady ? 'Uninstall this release' : helmReason}
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
      {data?.error !== undefined && (
        <p role="status" className="px-3 py-1 text-warn">
          {data.error}
        </p>
      )}

      {data !== null && (
        <div className="min-h-0 flex-1 overflow-auto">
          {tab === 'Overview' && (
            <div className="space-y-1 p-3">
              {data.release.fluxRef !== undefined && (
                <Field
                  label="Managed by"
                  value={`Flux ${data.release.fluxRef.namespace}/${data.release.fluxRef.name}`}
                />
              )}
              <Field label="Chart" value={orDash(data.release.chart)} />
              <Field label="Chart version" value={orDash(data.release.chartVersion)} />
              <Field label="App version" value={orDash(data.release.appVersion)} />
              <Field label="Latest" value={orDash(data.release.latest ?? '')} />
              <Field label="Revision" value={String(data.release.revision)} />
              <Field label="Status" value={orDash(data.release.status)} />
              <Field label="Description" value={orDash(data.release.description ?? '')} />
              <Field label="First deployed" value={orDash(data.firstDeployed ?? '')} />
              <Field label="Last deployed" value={orDash(data.release.updated)} />
              <Field label="Stored in" value={`${data.driver}s`} />
            </div>
          )}
          {tab === 'Values' && (
            <div>
              <div className="flex justify-end px-3 pt-2">
                <CopyButton what="release values" text={data.values} />
              </div>
              <Pane
                body={data.values}
                empty="This release was installed with the chart defaults."
              />
            </div>
          )}
          {tab === 'Notes' && <Pane body={data.notes} empty="This chart renders no notes." />}
          {tab === 'Manifest' && (
            <div>
              <div className="flex justify-end px-3 pt-2">
                <CopyButton what="release manifest" text={data.manifest} />
              </div>
              <Pane body={data.manifest} empty="This release rendered no manifest." />
            </div>
          )}
          {tab === 'Resources' && <Resources resources={data.resources} onOpen={onOpenResource} />}
          {tab === 'History' && (
            <History
              revisions={data.history}
              current={data.release.revision}
              now={now}
              busy={busy}
              helmReady={helmReady}
              helmReason={helmReason}
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
  helmReady,
  helmReason,
  onRollback,
}: {
  revisions: {
    revision: number;
    status: string;
    chartVersion: string;
    updated: string;
    description?: string;
  }[];
  current: number;
  now: number;
  busy: boolean;
  helmReady: boolean;
  helmReason: string;
  onRollback: (revision: number) => void;
}) {
  if (revisions.length === 0) {
    return <p className="p-3 text-fg-muted">This release has no stored revisions.</p>;
  }
  return (
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
            <td className="px-3 py-1 text-right">
              {entry.revision !== current && (
                <button
                  type="button"
                  disabled={busy || !helmReady}
                  title={helmReady ? `Roll back to revision ${String(entry.revision)}` : helmReason}
                  onClick={() => {
                    onRollback(entry.revision);
                  }}
                  className="rounded border border-edge-strong px-1.5 py-0.5 text-fg-soft hover:bg-surface-active disabled:cursor-not-allowed disabled:text-fg-faint"
                >
                  Roll back
                </button>
              )}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

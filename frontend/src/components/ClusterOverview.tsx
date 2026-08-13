import type { ReactNode } from 'react';
import type { NodeSummary, OverviewEvent, PodSummary } from '../lib/types';
import { percentOf, useOverview } from '../lib/overview';
import { cpuFromMilli, memFromMi } from '../lib/units';
import { ago } from '../lib/time';
import { useNow } from '../lib/useNow';
import LoadFailure from './LoadFailure';
import LoadWarning from './LoadWarning';
import StaleBanner from './StaleBanner';
import UsageBar from './UsageBar';

interface ClusterOverviewProps {
  active?: boolean;
}

function Tile({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div className="rounded border border-edge bg-surface-raised px-3 py-2">
      <div className="text-[11px] tracking-wide text-fg-muted uppercase">{label}</div>
      <div className="mt-0.5 text-lg text-fg-strong">{value}</div>
      {hint !== undefined && <div className="text-[11px] text-fg-muted">{hint}</div>}
    </div>
  );
}

function nodeHint(nodes: NodeSummary): string {
  const parts = [`${String(nodes.ready)} ready`];
  if (nodes.unschedulable > 0) {
    parts.push(`${String(nodes.unschedulable)} cordoned`);
  }
  const notReady = nodes.total - nodes.ready;
  if (notReady > 0) {
    parts.push(`${String(notReady)} not ready`);
  }
  return parts.join(' · ');
}

function podHint(pods: PodSummary): string {
  if (!pods.known) {
    return 'the tally could not be taken';
  }
  return [
    `${String(pods.running)} running`,
    `${String(pods.pending)} pending`,
    `${String(pods.failed)} failed`,
    `${String(pods.succeeded)} succeeded`,
  ].join(' · ');
}

function podTotal(pods: PodSummary): string {
  if (!pods.known) {
    return '—';
  }
  return String(pods.total);
}

function versionOf(version: string): string {
  if (version === '') {
    return 'unknown';
  }
  return version;
}

function Usage({
  label,
  used,
  total,
  known,
  format,
}: {
  label: string;
  used: number;
  total: number;
  known: boolean;
  format: (value: number) => string;
}) {
  const percent = percentOf(used, total);
  return (
    <div className="rounded border border-edge bg-surface-raised px-3 py-2">
      <div className="flex items-baseline justify-between">
        <span className="text-[11px] tracking-wide text-fg-muted uppercase">{label}</span>
        <span className="text-fg-soft">
          {usedLabel(used, known, format)} / {capacityLabel(total, format)}
        </span>
      </div>
      <div className="mt-1.5">
        <UsageBar percent={percent} label={`${label} ${String(percent)}%`} />
      </div>
    </div>
  );
}

function usedLabel(used: number, known: boolean, format: (value: number) => string): string {
  if (!known) {
    return '—';
  }
  if (used <= 0) {
    return '0';
  }
  return format(used);
}

function capacityLabel(total: number, format: (value: number) => string): string {
  if (total <= 0) {
    return '—';
  }
  return format(total);
}

function Warnings({ warnings, now }: { warnings: OverviewEvent[]; now: number }) {
  if (warnings.length === 0) {
    return <p className="px-1 text-fg-muted">No warning events in the cluster right now.</p>;
  }
  return (
    <table className="w-full border-collapse text-left">
      <thead className="text-fg-muted">
        <tr className="border-b border-edge">
          <th className="w-52 px-2 py-1 font-medium">Object</th>
          <th className="w-36 px-2 py-1 font-medium">Reason</th>
          <th className="px-2 py-1 font-medium">Message</th>
          <th className="w-14 px-2 py-1 text-right font-medium">Count</th>
          <th className="w-20 px-2 py-1 text-right font-medium">Last seen</th>
        </tr>
      </thead>
      <tbody>
        {warnings.map((warning) => (
          <tr key={eventKey(warning)} className="border-b border-edge align-top">
            <td className="truncate px-2 py-1 text-fg-strong" title={warning.namespace}>
              {warning.object}
              <span className="block truncate text-[11px] text-fg-muted">{warning.namespace}</span>
            </td>
            <td className="truncate px-2 py-1 text-warn">{warning.reason}</td>
            <td className="px-2 py-1 break-words text-fg-soft">{warning.message}</td>
            <td className="px-2 py-1 text-right text-fg-muted">{warning.count}</td>
            <td className="px-2 py-1 text-right text-fg-muted" title={warning.lastSeen}>
              {ago(warning.lastSeen, now)}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function eventKey(warning: OverviewEvent): string {
  return `${warning.namespace}/${warning.object}/${warning.reason}/${warning.lastSeen}`;
}

export default function ClusterOverview({ active = true }: ClusterOverviewProps) {
  const { data, error, reload } = useOverview(active);
  const now = useNow();

  if (data === null) {
    if (error !== null) {
      return <LoadFailure what="The cluster overview" message={error} />;
    }
    return (
      <div className="flex h-full items-center justify-center text-xs text-fg-muted">
        Loading the cluster overview…
      </div>
    );
  }

  let notice: ReactNode = null;
  if (error !== null) {
    notice = <StaleBanner what="The cluster overview" message={error} onRetry={reload} />;
  }

  return (
    <div className="flex h-full min-h-0 flex-col text-xs">
      {notice}
      {data.error !== undefined && <LoadWarning message={data.error} />}
      <div className="min-h-0 flex-1 overflow-auto p-3">
        <h2 className="mb-2 text-[11px] tracking-wide text-fg-muted uppercase">Cluster</h2>
        <div className="grid grid-cols-2 gap-2 lg:grid-cols-4">
          <Tile label="Kubernetes" value={versionOf(data.version)} />
          <Tile label="Nodes" value={String(data.nodes.total)} hint={nodeHint(data.nodes)} />
          <Tile label="Pods" value={podTotal(data.pods)} hint={podHint(data.pods)} />
          <Tile
            label="Recent warnings"
            value={String(data.warnings.length)}
            hint="the newest events shown"
          />
        </div>

        <h2 className="mt-4 mb-2 text-[11px] tracking-wide text-fg-muted uppercase">
          Allocatable capacity
        </h2>
        <div className="grid grid-cols-1 gap-2 lg:grid-cols-2">
          <Usage
            label="CPU"
            used={data.nodes.cpuUsedMilli}
            total={data.nodes.cpuAllocatableMilli}
            known={data.nodes.usageKnown}
            format={cpuFromMilli}
          />
          <Usage
            label="Memory"
            used={data.nodes.memUsedMi}
            total={data.nodes.memAllocatableMi}
            known={data.nodes.usageKnown}
            format={memFromMi}
          />
        </div>
        {!data.nodes.usageKnown && (
          <p className="mt-1.5 text-fg-muted">
            Live usage needs metrics-server; only the allocatable totals are shown.
          </p>
        )}

        <h2 className="mt-4 mb-2 text-[11px] tracking-wide text-fg-muted uppercase">
          Recent warnings
        </h2>
        <Warnings warnings={data.warnings} now={now} />
      </div>
    </div>
  );
}

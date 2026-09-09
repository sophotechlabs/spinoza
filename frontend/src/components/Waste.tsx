import { useState } from 'react';
import type { WasteRow } from '../lib/types';
import { cpuText, memText, rowKey, shareOf, useWaste } from '../lib/waste';
import { useShownCluster } from '../lib/tabs';
import { useNamespace } from '../store/namespace';
import WorkspaceHeader from './WorkspaceHeader';
import CapabilityState from './CapabilityState';
import DenseToolbar, { FilterInput, ToolbarCount } from './DenseToolbar';
import UsageBar from './UsageBar';

type Grouping = 'namespaces' | 'workloads';

const GROUP_LABELS: Record<Grouping, string> = {
  namespaces: 'By namespace',
  workloads: 'By workload',
};

function scopeLabel(cluster: string, namespace: string): string {
  if (namespace === '') {
    return cluster;
  }
  return `${cluster} · ${namespace}`;
}

function nameOf(row: WasteRow): string {
  if (row.name === undefined || row.name === '') {
    return row.namespace;
  }
  return `${row.namespace}/${row.name}`;
}

function matches(row: WasteRow, query: string): boolean {
  if (query === '') {
    return true;
  }
  return nameOf(row).toLowerCase().includes(query.toLowerCase());
}

function Row({ row, onOpen }: { row: WasteRow; onOpen: (row: WasteRow) => void }) {
  return (
    <tr className="border-b border-edge">
      <td className="px-3 py-1">
        <button
          type="button"
          onClick={() => {
            onOpen(row);
          }}
          className="truncate text-left text-fg hover:underline"
        >
          {nameOf(row)}
        </button>
        {row.kind !== undefined && row.kind !== '' && (
          <span className="ml-2 text-fg-muted">{row.kind}</span>
        )}
      </td>
      <td className="px-3 py-1 text-right font-mono text-fg-soft">{cpuText(row.cpuRequested)}</td>
      <td className="px-3 py-1">
        {row.measured === true ? (
          <UsageBar
            percent={shareOf(row.cpuUsed, row.cpuRequested) * 100}
            label={`CPU used by ${nameOf(row)}`}
            text={cpuText(row.cpuUsed)}
          />
        ) : (
          <span className="text-fg-subtle">not measured</span>
        )}
      </td>
      <td className="px-3 py-1 text-right font-mono text-warn">
        {row.measured === true ? cpuText(row.cpuReclaimable) : '—'}
      </td>
      <td className="px-3 py-1 text-right font-mono text-fg-soft">{memText(row.memRequested)}</td>
      <td className="px-3 py-1">
        {row.measured === true ? (
          <UsageBar
            percent={shareOf(row.memUsed, row.memRequested) * 100}
            label={`Memory used by ${nameOf(row)}`}
            text={memText(row.memUsed)}
          />
        ) : (
          <span className="text-fg-subtle">not measured</span>
        )}
      </td>
      <td className="px-3 py-1 text-right font-mono text-warn">
        {row.measured === true ? memText(row.memReclaimable) : '—'}
      </td>
    </tr>
  );
}

interface WasteProps {
  onOpenScope?: (namespace: string, kind: string) => void;
}

export default function Waste({ onOpenScope }: WasteProps) {
  const shownCluster = useShownCluster();
  const [grouping, setGrouping] = useState<Grouping>('namespaces');
  const [query, setQuery] = useState('');
  const scope = useNamespace();
  const { data: report, error, reload } = useWaste(scope);

  if (report === null) {
    if (error !== null) {
      return <CapabilityState state="failed" what="Reserved against used" why={error} />;
    }
    return <CapabilityState state="loading" what="what is reserved and never used" />;
  }

  const rows = (grouping === 'namespaces' ? report.namespaces : report.workloads).filter((one) =>
    matches(one, query),
  );
  const measured = report.measured === true;

  return (
    <div className="flex h-full min-h-0 flex-col text-xs">
      <WorkspaceHeader
        title="Reserved against used"
        scope={scopeLabel(shownCluster, scope)}
        scale={measured ? (report.window ?? '') : ''}
      />
      {error !== null && (
        <CapabilityState state="stale" what="Reserved against used" why={error} onRetry={reload} />
      )}
      {!measured && (
        <CapabilityState
          state="partial"
          why={report.reason ?? 'nothing measured what these workloads actually use'}
        />
      )}
      {measured && (report.partialOn ?? []).length > 0 && (
        <CapabilityState
          state="partial"
          why={`usage was not read for ${(report.partialOn ?? []).join(', ')}`}
        />
      )}
      <DenseToolbar label="Grouping">
        {(Object.keys(GROUP_LABELS) as Grouping[]).map((one) => (
          <button
            key={one}
            type="button"
            aria-pressed={grouping === one}
            onClick={() => {
              setGrouping(one);
            }}
            className={
              grouping === one
                ? 'border-accent-line text-accent rounded border px-2 py-0.5'
                : 'rounded border border-edge px-2 py-0.5 text-fg-muted hover:bg-surface-active'
            }
          >
            {GROUP_LABELS[one]}
          </button>
        ))}
        <FilterInput label="Filter rows" value={query} onChange={setQuery} />
        <ToolbarCount>{rows.length} rows</ToolbarCount>
        {report.source !== undefined && <ToolbarCount>usage from {report.source}</ToolbarCount>}
      </DenseToolbar>
      <div className="min-h-0 flex-1 overflow-auto">
        {rows.length === 0 && (
          <p className="p-3 text-fg-muted">Nothing here reserves CPU or memory.</p>
        )}
        {rows.length > 0 && (
          <table className="w-full table-fixed">
            <thead className="sticky top-0 bg-surface text-left text-fg-muted">
              <tr className="border-b border-edge">
                <th scope="col" className="px-3 py-1 font-normal">
                  {grouping === 'namespaces' ? 'Namespace' : 'Workload'}
                </th>
                <th scope="col" className="w-24 px-3 py-1 text-right font-normal">
                  CPU asked
                </th>
                <th scope="col" className="w-32 px-3 py-1 font-normal">
                  CPU used
                </th>
                <th scope="col" className="w-28 px-3 py-1 text-right font-normal">
                  CPU spare
                </th>
                <th scope="col" className="w-24 px-3 py-1 text-right font-normal">
                  Memory asked
                </th>
                <th scope="col" className="w-32 px-3 py-1 font-normal">
                  Memory used
                </th>
                <th scope="col" className="w-28 px-3 py-1 text-right font-normal">
                  Memory spare
                </th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => (
                <Row
                  key={rowKey(row)}
                  row={row}
                  onOpen={(chosen) => {
                    onOpenScope?.(chosen.namespace, chosen.kind ?? '');
                  }}
                />
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}

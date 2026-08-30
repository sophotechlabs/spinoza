import { useState } from 'react';
import type { ReactNode } from 'react';
import type { Issue, IssueChild, ObjectRef, Severity } from '../lib/types';
import {
  countBySeverity,
  foldedLabel,
  hiddenChildren,
  severityClass,
  severityLabel,
  usePagedIssues,
  ISSUE_ORDERS,
  orderLabel,
} from '../lib/issues';
import type { IssueOrder } from '../lib/issues';
import { ago } from '../lib/time';
import { useNow } from '../lib/useNow';
import LoadFailure from './LoadFailure';
import LoadWarning from './LoadWarning';
import StaleBanner from './StaleBanner';
import Loading from './Loading';
import { nameOf, tabOn, useClustersStore, useTabStrip } from '../store/clusters';
import { colorVar } from '../lib/clusterColor';
import { useFleetIssues, useIssuesStore } from '../store/issues';

interface IssueQueueProps {
  active?: boolean;
  onSelect?: (ref: ObjectRef) => void;
  onSelectOn?: (cluster: string, ref: ObjectRef) => void;
}

const SEVERITY_ORDER: Severity[] = ['fatal', 'degraded', 'warning'];

function Tally({ rows }: { rows: Issue[] }) {
  const counts = countBySeverity(rows);
  return (
    <div className="flex gap-3">
      {SEVERITY_ORDER.map((severity) => (
        <span key={severity} className={severityClass(severity)}>
          {counts[severity]} {severityLabel(severity).toLowerCase()}
        </span>
      ))}
    </div>
  );
}

function Where({ object, kind }: { object: ObjectRef; kind: string }) {
  return (
    <>
      <span className="text-fg-strong">{object.name}</span>
      <span className="block truncate text-[11px] text-fg-muted">
        {kind}
        {object.namespace !== '' && ` · ${object.namespace}`}
      </span>
    </>
  );
}

function OnCluster({ cluster }: { cluster: string }) {
  const tab = useClustersStore((state) => tabOn(state.tabs, cluster));
  if (tab === null) {
    return <span className="w-32 shrink-0 text-fg-faint">unknown</span>;
  }
  return (
    <span className="flex w-32 shrink-0 items-center gap-1.5 truncate text-fg-muted">
      <span
        aria-hidden="true"
        style={{ backgroundColor: colorVar(tab.color) }}
        className="h-2 w-2 shrink-0 rounded-sm"
      />
      <span className="truncate">{nameOf(tab)}</span>
    </span>
  );
}

function Change({ row, now }: { row: Issue; now: number }) {
  if (row.change === undefined || row.change === '') {
    return <span className="text-fg-faint">-</span>;
  }
  if (row.changedAt === undefined || row.changedAt === '') {
    return <span className="text-fg-muted">{row.change}</span>;
  }
  return (
    <span className="text-fg-muted" title={row.changedAt}>
      {row.change} · {ago(row.changedAt, now)}
    </span>
  );
}

function Children({ row, now }: { row: Issue; now: number }) {
  const children = row.children ?? [];
  const hidden = hiddenChildren(row);
  return (
    <div className="border-t border-edge bg-surface px-2 py-1">
      <ul>
        {children.map((child) => (
          <li key={childKey(child)} className="flex gap-2 py-0.5">
            <span className={`w-20 shrink-0 ${severityClass(child.severity)}`}>
              {severityLabel(child.severity)}
            </span>
            <span className="w-56 shrink-0 truncate text-fg-strong" title={child.object.namespace}>
              {child.object.name}
            </span>
            <span className="flex-1 break-words text-fg-soft">{child.detail}</span>
            <span className="w-16 shrink-0 text-right text-fg-muted" title={child.since}>
              {ago(child.since, now)}
            </span>
          </li>
        ))}
      </ul>
      {hidden > 0 && (
        <p className="py-0.5 text-fg-muted">
          and {hidden} more not listed, out of {row.folded}
        </p>
      )}
    </div>
  );
}

function emptyWord(fleet: boolean): string {
  if (fleet) {
    return 'Nothing is broken in any open cluster right now.';
  }
  return 'Nothing is broken in this cluster right now.';
}

function childKey(child: IssueChild): string {
  return `${child.object.namespace}/${child.object.resource}/${child.object.name}`;
}

function Row({
  row,
  now,
  open,
  fleet,
  onToggle,
  onSelect,
}: {
  row: Issue;
  now: number;
  open: boolean;
  fleet: boolean;
  onToggle: () => void;
  onSelect?: (ref: ObjectRef) => void;
}) {
  const folded = foldedLabel(row);
  return (
    <li className="border-b border-edge">
      <div className="flex items-start gap-2 px-2 py-1">
        <span className={`w-20 shrink-0 ${severityClass(row.severity)}`}>
          {severityLabel(row.severity)}
        </span>
        {fleet && <OnCluster cluster={row.cluster ?? ''} />}
        <button
          type="button"
          className="w-56 shrink-0 truncate text-left"
          onClick={() => {
            onSelect?.(row.object);
          }}
        >
          <Where object={row.object} kind={row.kind} />
        </button>
        <div className="min-w-0 flex-1">
          <div className="text-fg-strong">
            {row.title}
            {row.uncertain === true && (
              <span className="ml-2 text-[11px] text-fg-muted">(a guess)</span>
            )}
          </div>
          <div className="line-clamp-2 break-words text-fg-soft" title={row.detail}>
            {row.detail}
          </div>
          <div className="text-[11px] text-fg-muted">{row.action}</div>
        </div>
        <div className="w-44 shrink-0 text-right text-[11px]">
          <Change row={row} now={now} />
        </div>
        <div className="w-16 shrink-0 text-right text-fg-muted" title={row.since}>
          {ago(row.since, now)}
        </div>
        {folded === '' ? (
          <span className="w-24 shrink-0" />
        ) : (
          <button
            type="button"
            className="w-24 shrink-0 text-right text-fg-muted"
            aria-expanded={open}
            aria-label={`${open ? 'Hide' : 'Show'} the ${folded} folded under ${row.object.name}`}
            onClick={onToggle}
          >
            {open ? '▾' : '▸'} {folded}
          </button>
        )}
      </div>
      {open && row.folded > 0 && <Children row={row} now={now} />}
    </li>
  );
}

function moreLabel(loading: boolean): string {
  if (loading) {
    return 'Loading…';
  }
  return 'Show more';
}

export default function IssueQueue({ active = true, onSelect, onSelectOn }: IssueQueueProps) {
  const several = useTabStrip();
  const fleet = useFleetIssues();
  const setFleet = useIssuesStore((state) => state.setFleet);
  const showing = fleet && several;
  const [order, setOrder] = useState<IssueOrder>('worst');
  const { data, error, reload, rows, more, loadingMore, moreError, loadMore } = usePagedIssues(
    active,
    showing,
    order,
  );
  const [opened, setOpened] = useState<Record<string, boolean>>({});
  const now = useNow();

  function pick(row: Issue) {
    if (showing && row.cluster !== undefined && row.cluster !== '') {
      onSelectOn?.(row.cluster, row.object);
      return;
    }
    onSelect?.(row.object);
  }

  if (data === null) {
    if (error !== null) {
      return <LoadFailure what="The issue queue" message={error} />;
    }
    return <Loading what="the issue queue" />;
  }

  let notice: ReactNode = null;
  if (error !== null) {
    notice = <StaleBanner what="The issue queue" message={error} onRetry={reload} />;
  }

  function toggle(id: string) {
    setOpened((current) => ({ ...current, [id]: !current[id] }));
  }

  return (
    <div className="flex h-full min-h-0 flex-col text-xs">
      {notice}
      {data.error !== undefined && <LoadWarning message={data.error} />}
      <div className="flex items-center justify-between border-b border-edge px-2 py-1.5">
        <div className="flex items-center gap-3">
          <h2 className="text-[11px] tracking-wide text-fg-muted uppercase">Issues</h2>
          {several && (
            <label className="flex items-center gap-1.5 text-fg-soft">
              <input
                type="checkbox"
                checked={fleet}
                onChange={(event) => {
                  setFleet(event.target.checked);
                }}
              />
              Every open cluster
            </label>
          )}
        </div>
        <label className="flex items-center gap-1.5 text-fg-soft">
          Sort
          <select
            aria-label="Sort issues"
            value={order}
            onChange={(event) => {
              setOrder(event.target.value as IssueOrder);
            }}
            className="rounded border border-edge-strong bg-surface-raised px-1 py-0.5 text-fg"
          >
            {ISSUE_ORDERS.map((one) => (
              <option key={one} value={one}>
                {orderLabel(one)}
              </option>
            ))}
          </select>
        </label>
        <Tally rows={rows} />
      </div>
      <div className="min-h-0 flex-1 overflow-auto">
        {rows.length === 0 && <p className="p-3 text-fg-muted">{emptyWord(showing)}</p>}
        <ul>
          {rows.map((row) => (
            <Row
              key={row.id}
              row={row}
              now={now}
              open={opened[row.id] ?? false}
              onToggle={() => {
                toggle(row.id);
              }}
              fleet={showing}
              onSelect={() => {
                pick(row);
              }}
            />
          ))}
        </ul>
        {more !== '' && (
          <button
            type="button"
            disabled={loadingMore}
            onClick={loadMore}
            className="w-full border-t border-edge p-2 text-fg-muted hover:text-fg-strong disabled:text-fg-faint"
          >
            {moreLabel(loadingMore)}
          </button>
        )}
        {moreError !== '' && <p className="p-3 text-error">{moreError}</p>}
        {data.dropped > 0 && (
          <p className="p-3 text-fg-muted">
            {data.dropped} more rows are not shown; the queue stops at the {rows.length} worst.
          </p>
        )}
      </div>
    </div>
  );
}

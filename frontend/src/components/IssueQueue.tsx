import { useState } from 'react';
import type { ReactNode } from 'react';
import type { Issue, IssueChild, ObjectRef, Severity } from '../lib/types';
import {
  countBySeverity,
  foldedLabel,
  hiddenChildren,
  severityClass,
  severityLabel,
  useIssues,
} from '../lib/issues';
import { ago } from '../lib/time';
import { useNow } from '../lib/useNow';
import LoadFailure from './LoadFailure';
import LoadWarning from './LoadWarning';
import StaleBanner from './StaleBanner';
import Loading from './Loading';

interface IssueQueueProps {
  active?: boolean;
  onSelect?: (ref: ObjectRef) => void;
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

function childKey(child: IssueChild): string {
  return `${child.object.namespace}/${child.object.resource}/${child.object.name}`;
}

function Row({
  row,
  now,
  open,
  onToggle,
  onSelect,
}: {
  row: Issue;
  now: number;
  open: boolean;
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

export default function IssueQueue({ active = true, onSelect }: IssueQueueProps) {
  const { data, error, reload } = useIssues(active);
  const [opened, setOpened] = useState<Record<string, boolean>>({});
  const now = useNow();

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
        <h2 className="text-[11px] tracking-wide text-fg-muted uppercase">Issues</h2>
        <Tally rows={data.rows} />
      </div>
      <div className="min-h-0 flex-1 overflow-auto">
        {data.rows.length === 0 && (
          <p className="p-3 text-fg-muted">Nothing is broken in this cluster right now.</p>
        )}
        <ul>
          {data.rows.map((row) => (
            <Row
              key={row.id}
              row={row}
              now={now}
              open={opened[row.id] ?? false}
              onToggle={() => {
                toggle(row.id);
              }}
              onSelect={onSelect}
            />
          ))}
        </ul>
        {data.dropped > 0 && (
          <p className="p-3 text-fg-muted">
            {data.dropped} more rows are not shown; the queue stops at the {data.rows.length} worst.
          </p>
        )}
      </div>
    </div>
  );
}

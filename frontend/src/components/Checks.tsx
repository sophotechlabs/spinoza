import { useState } from 'react';
import type { CheckCategory, ObjectRef } from '../lib/types';
import type { CheckFindingView, CheckGroupView } from '../lib/checks';
import { fetchCheckPage } from '../lib/checks';
import {
  CATEGORY_LABELS,
  CATEGORY_ORDER,
  countLabel,
  findingLabel,
  inCategory,
  originLabel,
  severityClass,
  shownLabel,
  totalFindings,
  useChecks,
} from '../lib/checks';
import type { SeverityFloor } from '../lib/settings';
import { useSettingsStore } from '../store/settings';
import LoadFailure from './LoadFailure';
import StaleBanner from './StaleBanner';
import Loading from './Loading';

const PAGE_SIZE = 200;

interface ChecksProps {
  onOpen: (ref: ObjectRef, kind: string) => void;
}

function messageOf(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'the findings request failed';
}

function moreLabel(loading: boolean, left: number): string {
  if (loading) {
    return 'Loading';
  }
  return `Show ${String(Math.min(left, PAGE_SIZE))} more`;
}

function chevron(open: boolean): string {
  if (open) {
    return '▾';
  }
  return '▸';
}

function countClass(group: CheckGroupView): string {
  if (group.skipped !== undefined) {
    return 'text-fg-subtle';
  }
  if (group.total === 0) {
    return 'text-ok';
  }
  return severityClass(group.severity);
}

function scannedLabel(scanned: number, findings: number): string {
  return `${String(findings)} findings across ${String(scanned)} workloads`;
}

function Finding({
  finding,
  onOpen,
}: {
  finding: CheckFindingView;
  onOpen: (ref: ObjectRef, kind: string) => void;
}) {
  return (
    <li className="border-t border-edge px-3 py-1.5 pl-9">
      <div className="flex items-baseline gap-3">
        <button
          type="button"
          onClick={() => {
            onOpen(finding.object, finding.kind);
          }}
          className="min-w-0 shrink-0 truncate text-fg-strong hover:underline"
        >
          {findingLabel(finding)}
        </button>
        <span className="min-w-0 flex-1 truncate text-fg-muted" title={finding.detail}>
          {finding.detail}
        </span>
        {originLabel(finding) !== '' && (
          <span className="shrink-0 rounded border border-edge px-1 text-[10px] text-fg-soft">
            {originLabel(finding)}
          </span>
        )}
      </div>
      {finding.patch !== undefined && (
        <pre className="mt-1 overflow-x-auto rounded border border-edge bg-surface-raised px-2 py-1 text-[11px] text-fg-soft">
          {finding.patch}
        </pre>
      )}
    </li>
  );
}

function Group({
  group,
  onOpen,
}: {
  group: CheckGroupView;
  onOpen: (ref: ObjectRef, kind: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [paged, setPaged] = useState<CheckFindingView[] | null>(null);
  const [cursor, setCursor] = useState('');
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState<string | null>(null);
  const empty = group.findings.length === 0;
  const shown = paged ?? group.findings;
  const nextCursor = paged === null ? (group.next ?? '') : cursor;

  function toggle() {
    if (open) {
      setPaged(null);
      setCursor('');
      setFailed(null);
    }
    setOpen(!open);
  }

  async function loadMore() {
    setLoading(true);
    setFailed(null);
    try {
      const page = await fetchCheckPage(group.id, nextCursor);
      setPaged([...shown, ...page.findings]);
      setCursor(page.next);
    } catch (err: unknown) {
      setFailed(messageOf(err));
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="border-t border-edge">
      <div className="flex items-baseline pr-3 hover:bg-surface-raised">
        <button
          type="button"
          disabled={empty}
          aria-expanded={!empty && open}
          onClick={toggle}
          className="flex min-w-0 flex-1 items-baseline gap-3 px-3 py-1.5 text-left"
        >
          <span aria-hidden="true" className="w-3 shrink-0 text-fg-subtle">
            {!empty && chevron(open)}
          </span>
          <span className="min-w-0 flex-1 truncate text-fg-strong">{group.title}</span>
          {(group.frameworks ?? []).map((framework) => (
            <span key={framework} className="shrink-0 text-[11px] text-fg-subtle">
              {framework}
            </span>
          ))}
          <span className={`w-16 shrink-0 text-right ${severityClass(group.severity)}`}>
            {group.severity}
          </span>
          <span className={`w-16 shrink-0 text-right ${countClass(group)}`}>
            {countLabel(group)}
          </span>
        </button>
        <TurnOff id={group.id} />
      </div>
      {open && !empty && (
        <div className="pb-1">
          <p className="px-3 py-1 pl-9 text-fg-muted">{group.wrong}</p>
          <p className="px-3 py-1 pl-9 text-fg-soft">{group.remedy}</p>
          {shownLabel(group, shown.length) !== '' && (
            <p className="px-3 py-1 pl-9 text-fg-subtle">{shownLabel(group, shown.length)}</p>
          )}
          <ul>
            {shown.map((finding) => (
              <Finding
                key={`${finding.object.namespace}/${finding.object.name}/${finding.container ?? ''}`}
                finding={finding}
                onOpen={onOpen}
              />
            ))}
          </ul>
          {failed !== null && (
            <p role="alert" className="px-3 py-1 pl-9 text-error">
              {failed}
            </p>
          )}
          {nextCursor !== '' && (
            <button
              type="button"
              disabled={loading}
              onClick={() => {
                void loadMore();
              }}
              className="mt-1 ml-9 rounded border border-edge-strong px-2 py-0.5 text-fg-soft hover:bg-surface-raised disabled:text-fg-subtle"
            >
              {moreLabel(loading, group.total - shown.length)}
            </button>
          )}
        </div>
      )}
      {group.skipped !== undefined && (
        <p className="px-3 py-1 pl-9 text-fg-subtle">{group.skipped}</p>
      )}
    </div>
  );
}

function namesFrom(raw: string): string[] {
  return raw
    .split(',')
    .map((part) => part.trim())
    .filter((part) => part !== '');
}

function AuditControls() {
  const floor = useSettingsStore((state) => state.checksMinSeverity);
  const setFloor = useSettingsStore((state) => state.setChecksMinSeverity);
  const wholeCluster = useSettingsStore((state) => state.checksWholeCluster);
  const setWholeCluster = useSettingsStore((state) => state.setChecksWholeCluster);
  const skipped = useSettingsStore((state) => state.checksSkipNamespaces);
  const setSkipped = useSettingsStore((state) => state.setChecksSkipNamespaces);
  const [typing, setTyping] = useState(skipped.join(','));
  const off = useSettingsStore((state) => state.checksDisabled);
  const setOff = useSettingsStore((state) => state.setChecksDisabled);

  return (
    <div className="flex shrink-0 items-center gap-4 border-b border-edge px-3 py-1.5 text-fg-muted">
      <label className="flex items-center gap-1.5">
        Show
        <select
          aria-label="Lowest severity to show"
          className="rounded border border-edge bg-surface px-1 py-0.5 text-fg"
          value={floor}
          onChange={(event) => {
            setFloor(event.target.value as SeverityFloor);
          }}
        >
          <option value="">everything</option>
          <option value="medium">medium and above</option>
          <option value="high">high only</option>
        </select>
      </label>
      <label className="flex items-center gap-1.5">
        Skip
        <input
          type="text"
          aria-label="Namespaces to skip"
          placeholder="namespaces, comma separated"
          className="w-56 rounded border border-edge bg-surface px-1 py-0.5 text-fg"
          value={typing}
          onChange={(event) => {
            setTyping(event.target.value);
            setSkipped(namesFrom(event.target.value));
          }}
        />
      </label>
      <label className="flex items-center gap-1.5">
        <input
          type="checkbox"
          checked={wholeCluster}
          onChange={(event) => {
            setWholeCluster(event.target.checked);
          }}
        />
        Audit the whole cluster
      </label>
      {off.length > 0 && (
        <button
          type="button"
          className="ml-auto hover:underline"
          onClick={() => {
            setOff([]);
          }}
        >
          {off.length} turned off · turn back on
        </button>
      )}
    </div>
  );
}

function TurnOff({ id }: { id: string }) {
  const off = useSettingsStore((state) => state.checksDisabled);
  const setOff = useSettingsStore((state) => state.setChecksDisabled);

  return (
    <button
      type="button"
      aria-label={`Turn off ${id}`}
      className="ml-3 shrink-0 text-fg-soft hover:underline"
      onClick={() => {
        setOff([...off, id]);
      }}
    >
      off
    </button>
  );
}

function Category({
  category,
  groups,
  onOpen,
}: {
  category: CheckCategory;
  groups: CheckGroupView[];
  onOpen: (ref: ObjectRef, kind: string) => void;
}) {
  if (groups.length === 0) {
    return null;
  }
  return (
    <section className="mb-3">
      <h2 className="px-3 py-1 text-[11px] font-semibold tracking-wide text-fg-muted uppercase">
        {CATEGORY_LABELS[category]}
      </h2>
      {groups.map((group) => (
        <Group key={group.id} group={group} onOpen={onOpen} />
      ))}
    </section>
  );
}

export default function Checks({ onOpen }: ChecksProps) {
  const { data, error, stale, reload } = useChecks();

  if (data === null) {
    if (error !== null) {
      return <LoadFailure what="The cluster audit" message={error} />;
    }
    return <Loading what="the cluster audit" />;
  }

  return (
    <div className="flex h-full min-h-0 flex-col text-xs">
      {stale && error !== null && (
        <StaleBanner what="The cluster audit" message={error} onRetry={reload} />
      )}
      {data.error !== undefined && (
        <p role="status" className="border-b border-edge px-3 py-1 text-warn">
          {data.error}
        </p>
      )}
      <AuditControls />
      <div className="flex shrink-0 items-baseline gap-3 border-b border-edge px-3 py-1.5 text-fg-muted">
        <span className="min-w-0 flex-1">{scannedLabel(data.scanned, totalFindings(data))}</span>
        <span className="w-16 shrink-0 text-right">Severity</span>
        <span className="w-16 shrink-0 text-right">Findings</span>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto">
        {CATEGORY_ORDER.map((category) => (
          <Category
            key={category}
            category={category}
            groups={inCategory(data.groups, category)}
            onOpen={onOpen}
          />
        ))}
      </div>
    </div>
  );
}

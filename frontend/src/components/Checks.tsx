import { useEffect, useRef, useState } from 'react';
import type { CheckCategory, Mute, NamespaceCount, ObjectRef, RuleFault } from '../lib/types';
import type { CheckFindingView, CheckGroupView, CheckReportView } from '../lib/checks';
import { fetchCheckPage } from '../lib/checks';
import {
  CATEGORY_LABELS,
  CATEGORY_ORDER,
  changeLabel,
  clearBaseline,
  driftLabel,
  countLabel,
  partialLabel,
  exportChecks,
  fetchMutes,
  findingLabel,
  inCategory,
  muteFinding,
  mutedLabel,
  originLabel,
  loadBaselineFile,
  refKeyOf,
  ruleFaults,
  saveBaselineFile,
  severityClass,
  severityReason,
  shownLabel,
  takeBaseline,
  totalFindings,
  unmuteFinding,
  useChecks,
} from '../lib/checks';
import type { SeverityFloor } from '../lib/settings';
import { useChecksFilter, useSettingsStore } from '../store/settings';
import type { ChecksFilter } from '../store/settings';
import LoadFailure from './LoadFailure';
import StaleBanner from './StaleBanner';
import Loading from './Loading';
import { nameOf, tabOn, useClustersStore, useTabStrip } from '../store/clusters';
import { colorVar } from '../lib/clusterColor';
import { useContextScope } from '../store/contexts';

const PAGE_SIZE = 200;

const NAMESPACES_SHOWN = 20;

function useScopedOperation() {
  const operation = useRef(0);
  useEffect(() => {
    return () => {
      operation.current += 1;
    };
  }, []);
  return operation;
}

function baselineLabel(report: CheckReportView): string {
  if (report.baseline === '') {
    return 'No baseline taken, so nothing is marked new.';
  }
  const day = report.baseline.slice(0, 10);
  if (report.baselineFrom === '') {
    return `Comparing against ${day}.`;
  }
  return (
    `Comparing against ${report.baselineFrom}, ${day}: ` +
    `${String(report.wasScanned)} workloads there, ${String(report.scanned)} here. ` +
    `Different objects, so counts rather than what is new.`
  );
}

function takeLabel(baseline: string, working: boolean): string {
  if (working) {
    return 'Working';
  }
  if (baseline === '') {
    return 'Take a baseline';
  }
  return 'Take a new one';
}

function namespacesLabel(counts: NamespaceCount[], picked: string): string {
  if (picked !== '') {
    return `Showing ${picked} only`;
  }
  return `${String(counts.length)} namespaces with findings`;
}

function clustersLabel(clusters: string[] | undefined): string {
  if (clusters === undefined || clusters.length === 0) {
    return '';
  }
  return clusters.join(', ');
}

function pickedNext(picked: string, namespace: string): string {
  if (picked === namespace) {
    return '';
  }
  return namespace;
}

interface ChecksProps {
  onOpen: (ref: ObjectRef, kind: string) => void;
}

function messageOf(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'the findings request failed';
}

function moreLabel(loading: boolean, left: number, fleet: boolean): string {
  if (loading) {
    return 'Loading';
  }
  if (fleet) {
    return 'Show more';
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

function scannedLabel(scanned: number, findings: number, namespace: string): string {
  if (namespace !== '') {
    return `${String(findings)} findings in ${namespace}`;
  }
  return `${String(findings)} findings across ${String(scanned)} workloads`;
}

function OnCluster({ cluster }: { cluster: string }) {
  const tab = useClustersStore((state) => tabOn(state.tabs, cluster));
  if (tab === null) {
    return <span className="shrink-0 text-fg-faint">unknown</span>;
  }
  return (
    <span className="flex shrink-0 items-center gap-1.5 text-fg-muted">
      <span
        aria-hidden="true"
        style={{ backgroundColor: colorVar(tab.color) }}
        className="h-2 w-2 shrink-0 rounded-sm"
      />
      {nameOf(tab)}
    </span>
  );
}

function Finding({
  finding,
  check,
  onOpen,
  onChanged,
}: {
  finding: CheckFindingView;
  check: string;
  onOpen: (ref: ObjectRef, kind: string) => void;
  onChanged: () => void;
}) {
  const [asking, setAsking] = useState(false);
  const [failed, setFailed] = useState<string | null>(null);

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
        {finding.cluster !== undefined && finding.cluster !== '' && (
          <OnCluster cluster={finding.cluster} />
        )}
        <span className="min-w-0 flex-1 truncate text-fg-muted" title={finding.detail}>
          {finding.detail}
        </span>
        {finding.fresh && <span className="shrink-0 text-[10px] text-warn">new</span>}
        {originLabel(finding) !== '' && (
          <span className="shrink-0 rounded border border-edge px-1 text-[10px] text-fg-soft">
            {originLabel(finding)}
          </span>
        )}
        <span
          title={severityReason(finding)}
          className={`w-16 shrink-0 text-right text-[11px] ${severityClass(finding.severity)}`}
        >
          {finding.severity}
        </span>
        <MuteControl
          finding={finding}
          check={check}
          asking={asking}
          onAsk={setAsking}
          onChanged={onChanged}
          onFailed={setFailed}
        />
      </div>
      {failed !== null && <p className="pt-0.5 text-[11px] text-warn">{failed}</p>}
      {finding.muted && finding.reason !== undefined && finding.reason !== '' && (
        <p className="pt-0.5 text-[11px] text-fg-subtle">muted: {finding.reason}</p>
      )}
      {asking && !finding.muted && (
        <MuteReason
          check={check}
          finding={finding}
          onDone={() => {
            setAsking(false);
            onChanged();
          }}
        />
      )}
      {finding.patch !== undefined && (
        <pre className="mt-1 overflow-x-auto rounded border border-edge bg-surface-raised px-2 py-1 text-[11px] text-fg-soft">
          {finding.patch}
        </pre>
      )}
    </li>
  );
}

function MuteControl({
  finding,
  check,
  asking,
  onAsk,
  onChanged,
  onFailed,
}: {
  finding: CheckFindingView;
  check: string;
  asking: boolean;
  onAsk: (asking: boolean) => void;
  onChanged: () => void;
  onFailed: (message: string) => void;
}) {
  const scope = useContextScope();
  const operation = useScopedOperation();
  const [working, setWorking] = useState(false);

  useEffect(() => {
    setWorking(false);
  }, [scope]);

  if (finding.mutedBy === 'convention' || finding.mutedBy === 'rule') {
    return <span className="shrink-0 text-fg-subtle">{silencedLabel(finding.mutedBy)}</span>;
  }
  if (finding.muted) {
    return (
      <button
        type="button"
        disabled={working}
        aria-label={`Unmute ${findingLabel(finding)}`}
        className="shrink-0 text-fg-soft hover:underline disabled:text-fg-subtle"
        onClick={() => {
          operation.current += 1;
          const token = operation.current;
          setWorking(true);
          unmuteFinding({ check, ...refFor(scopeOf(finding), finding) })
            .then(() => {
              if (operation.current === token) {
                onChanged();
              }
            })
            .catch((err: unknown) => {
              if (operation.current === token) {
                onFailed(messageOf(err));
              }
            })
            .finally(() => {
              if (operation.current === token) {
                setWorking(false);
              }
            });
        }}
      >
        unmute
      </button>
    );
  }
  return (
    <button
      type="button"
      aria-label={`Mute ${findingLabel(finding)}`}
      className="shrink-0 text-fg-soft hover:underline"
      onClick={() => {
        onAsk(!asking);
      }}
    >
      mute
    </button>
  );
}

function MuteReason({
  check,
  finding,
  onDone,
}: {
  check: string;
  finding: CheckFindingView;
  onDone: () => void;
}) {
  const [reason, setReason] = useState('');
  const [failed, setFailed] = useState<string | null>(null);
  const [working, setWorking] = useState(false);
  const clusterScope = useContextScope();
  const operation = useScopedOperation();

  useEffect(() => {
    setWorking(false);
    setFailed(null);
  }, [clusterScope]);

  function save(scope: MuteScope) {
    operation.current += 1;
    const token = operation.current;
    setWorking(true);
    const mute = { check, reason, ...refFor(scope, finding) };
    muteFinding(mute)
      .then(() => {
        if (operation.current === token) {
          onDone();
        }
      })
      .catch((err: unknown) => {
        if (operation.current === token) {
          setFailed(messageOf(err));
        }
      })
      .finally(() => {
        if (operation.current === token) {
          setWorking(false);
        }
      });
  }

  return (
    <div className="flex items-center gap-2 pt-1">
      <input
        type="text"
        aria-label="Why this one is being muted"
        placeholder="why you have decided about this"
        className="w-72 rounded border border-edge bg-surface px-1 py-0.5 text-fg"
        value={reason}
        onChange={(event) => {
          setReason(event.target.value);
        }}
      />
      <button
        type="button"
        disabled={working}
        className="rounded border border-edge px-2 py-0.5 text-fg-strong"
        onClick={() => {
          save('object');
        }}
      >
        Mute this one
      </button>
      {finding.object.namespace !== '' && (
        <button
          type="button"
          disabled={working}
          className="rounded border border-edge px-2 py-0.5 text-fg-soft"
          onClick={() => {
            save('namespace');
          }}
        >
          Mute in {finding.object.namespace}
        </button>
      )}
      {failed !== null && <span className="text-warn">{failed}</span>}
    </div>
  );
}

function silencedLabel(by: string): string {
  if (by === 'rule') {
    return 'by a rule';
  }
  return 'silenced';
}

type MuteScope = 'object' | 'namespace' | 'check';

function scopeOf(finding: CheckFindingView): MuteScope {
  if (finding.mutedBy === 'namespace') {
    return 'namespace';
  }
  if (finding.mutedBy === 'check') {
    return 'check';
  }
  return 'object';
}

function refFor(scope: MuteScope, finding: CheckFindingView): Partial<Mute> {
  if (scope === 'namespace') {
    return { namespace: finding.object.namespace };
  }
  if (scope === 'check') {
    return {};
  }
  return { ref: refKeyOf(finding.object) };
}

function driftOrChange(group: CheckGroupView, report: CheckReportView): string {
  if (report.baseline === '') {
    return '';
  }
  if (report.baselineFrom === '') {
    return changeLabel(group);
  }
  if (group.measured === true) {
    return 'measured, not compared';
  }
  return driftLabel(group, report.scanned, report.wasScanned);
}

function Group({
  group,
  report,
  keep,
  fleet,
  onOpen,
  onChanged,
}: {
  group: CheckGroupView;
  report: CheckReportView;
  keep: ChecksFilter;
  fleet: boolean;
  onOpen: (ref: ObjectRef, kind: string) => void;
  onChanged: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [paged, setPaged] = useState<CheckFindingView[] | null>(null);
  const [cursor, setCursor] = useState('');
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState<string | null>(null);
  const pageRequest = useRef(0);
  const empty = group.findings.length === 0;
  const shown = paged ?? group.findings;
  const nextCursor = paged === null ? (group.next ?? '') : cursor;
  const pageScope = JSON.stringify({ group, keep, fleet });

  useEffect(() => {
    pageRequest.current += 1;
    setPaged(null);
    setCursor('');
    setLoading(false);
    setFailed(null);
  }, [pageScope]);

  useEffect(() => {
    return () => {
      pageRequest.current += 1;
    };
  }, []);

  function toggle() {
    if (open) {
      pageRequest.current += 1;
      setPaged(null);
      setCursor('');
      setLoading(false);
      setFailed(null);
    }
    setOpen(!open);
  }

  async function loadMore() {
    pageRequest.current += 1;
    const token = pageRequest.current;
    setLoading(true);
    setFailed(null);
    try {
      const page = await fetchCheckPage(group.id, nextCursor, keep, fleet);
      if (pageRequest.current !== token) {
        return;
      }
      setPaged([...shown, ...page.findings]);
      setCursor(page.next);
    } catch (err: unknown) {
      if (pageRequest.current !== token) {
        return;
      }
      setFailed(messageOf(err));
    } finally {
      if (pageRequest.current === token) {
        setLoading(false);
      }
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
          {driftOrChange(group, report) !== '' && (
            <span className="shrink-0 text-[11px] text-warn">{driftOrChange(group, report)}</span>
          )}
          {mutedLabel(group) !== '' && (
            <span className="shrink-0 text-[11px] text-fg-subtle">{mutedLabel(group)}</span>
          )}
          {partialLabel(group) !== '' && (
            <span className="shrink-0 text-[11px] text-warn">{partialLabel(group)}</span>
          )}
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
          {(group.partialOn ?? []).map((stood) => (
            <p key={stood} className="px-3 py-1 pl-9 text-warn">
              {stood}
            </p>
          ))}
          {shownLabel(group, shown.length) !== '' && (
            <p className="px-3 py-1 pl-9 text-fg-subtle">{shownLabel(group, shown.length)}</p>
          )}
          <Gone gone={group.gone ?? []} />
          <ul>
            {shown.map((finding) => (
              <Finding
                key={`${finding.object.namespace}/${finding.object.name}/${finding.container ?? ''}`}
                finding={finding}
                check={group.id}
                onOpen={onOpen}
                onChanged={onChanged}
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
              {moreLabel(loading, group.total - shown.length, fleet)}
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

function Gone({ gone }: { gone: string[] }) {
  if (gone.length === 0) {
    return null;
  }
  return (
    <details className="px-3 py-1 pl-9 text-fg-subtle">
      <summary className="cursor-pointer">{goneLabel(gone.length)}</summary>
      <ul className="py-1">
        {gone.map((one) => (
          <li key={one}>{one}</li>
        ))}
      </ul>
    </details>
  );
}

function goneLabel(count: number): string {
  if (count === 1) {
    return 'One that was here at the baseline and is not now';
  }
  return `${String(count)} that were here at the baseline and are not now`;
}

function namesFrom(raw: string): string[] {
  return raw
    .split(',')
    .map((part) => part.trim())
    .filter((part) => part !== '');
}

function YourRules() {
  const saved = useSettingsStore((state) => state.checkRules);
  const save = useSettingsStore((state) => state.setCheckRules);
  const [draft, setDraft] = useState(saved);
  const [failed, setFailed] = useState<string | null>(null);
  const [faults, setFaults] = useState<RuleFault[] | null>(null);
  const [copied, setCopied] = useState(false);
  const dirty = draft !== saved;

  function check(): Promise<RuleFault[]> {
    return ruleFaults(draft).then((found) => {
      setFaults(found);
      return found;
    });
  }

  return (
    <details className="shrink-0 border-b border-edge px-3 py-1.5 text-fg-muted">
      <summary className="cursor-pointer">Your own rules</summary>
      <p className="py-1 text-fg-soft">
        A list of {'{ id, match, expr }'} objects. The expression is CEL, with the workload bound to
        object, and a rule that matches becomes a finding. Give a rule {'{ silences, reason }'}{' '}
        instead and it quietens that check where it matches, saying why. Copy the list to keep it in
        a repository, and paste one back here to use it on another cluster. If a field is absent,
        the expression does not match that object and the audit names the evaluation fault.
      </p>
      <textarea
        aria-label="Your own rules"
        spellCheck={false}
        rows={6}
        className="w-full rounded border border-edge bg-surface px-2 py-1 font-mono text-[11px] text-fg"
        value={draft}
        onChange={(event) => {
          setDraft(event.target.value);
          setFaults(null);
          setCopied(false);
        }}
      />
      <div className="flex items-center gap-3 py-1">
        <button
          type="button"
          disabled={!dirty}
          className="rounded border border-edge px-2 py-0.5 text-fg-strong disabled:text-fg-subtle"
          onClick={() => {
            check()
              .then((found) => {
                if (found.length > 0) {
                  return;
                }
                save(draft)
                  .then(() => {
                    setFailed(null);
                  })
                  .catch((reason: unknown) => {
                    setFailed(messageOf(reason));
                  });
              })
              .catch((reason: unknown) => {
                setFailed(messageOf(reason));
              });
          }}
        >
          Save
        </button>
        <button
          type="button"
          className="rounded border border-edge px-2 py-0.5 text-fg-soft"
          onClick={() => {
            void check().catch((reason: unknown) => {
              setFailed(messageOf(reason));
            });
          }}
        >
          Check
        </button>
        <button
          type="button"
          disabled={draft === ''}
          className="rounded border border-edge px-2 py-0.5 text-fg-soft disabled:text-fg-subtle"
          onClick={() => {
            navigator.clipboard
              .writeText(draft)
              .then(() => {
                setCopied(true);
              })
              .catch(() => {
                setFailed('the rules could not be copied');
              });
          }}
        >
          {copyLabel(copied)}
        </button>
        {failed !== null && <span className="text-warn">{failed}</span>}
      </div>
      <RuleFaults faults={faults} />
    </details>
  );
}

function copyLabel(copied: boolean): string {
  if (copied) {
    return 'Copied';
  }
  return 'Copy';
}

function RuleFaults({ faults }: { faults: RuleFault[] | null }) {
  if (faults === null) {
    return null;
  }
  if (faults.length === 0) {
    return <p className="py-1 text-ok">Every rule reads.</p>;
  }
  return (
    <ul className="py-1">
      {faults.map((fault) => (
        <li key={fault.id + fault.reason} className="text-warn">
          {faultLabel(fault)}
        </li>
      ))}
    </ul>
  );
}

function faultLabel(fault: RuleFault): string {
  if (fault.id === '') {
    return fault.reason;
  }
  return `${fault.id}: ${fault.reason}`;
}

function saveAs(name: string, body: Blob): void {
  const url = URL.createObjectURL(body);
  const link = document.createElement('a');
  link.href = url;
  link.download = name;
  link.click();
  URL.revokeObjectURL(url);
}

function MutesPanel({ audit, onChanged }: { audit: unknown; onChanged: () => void }) {
  const [open, setOpen] = useState(false);
  const [mutes, setMutes] = useState<Mute[] | null>(null);
  const [failed, setFailed] = useState<string | null>(null);
  const latestAudit = useRef(audit);
  const mounted = useRef(true);
  latestAudit.current = audit;

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  useEffect(() => {
    if (!open) {
      return;
    }
    let live = true;
    fetchMutes()
      .then((found) => {
        if (!live) {
          return;
        }
        setMutes(found);
        setFailed(null);
      })
      .catch((err: unknown) => {
        if (live) {
          setFailed(messageOf(err));
        }
      });
    return () => {
      live = false;
    };
  }, [open, audit]);

  return (
    <details
      className="shrink-0 border-b border-edge px-3 py-1.5 text-fg-muted"
      onToggle={(event) => {
        setOpen(event.currentTarget.open);
      }}
    >
      <summary className="cursor-pointer">What you have muted</summary>
      {failed !== null && <p className="py-1 text-warn">{failed}</p>}
      {mutes !== null && mutes.length === 0 && (
        <p className="py-1 text-fg-soft">You have not muted anything on this cluster.</p>
      )}
      <ul className="py-1">
        {(mutes ?? []).map((mute) => (
          <li key={muteKey(mute)} className="flex items-baseline gap-3 py-0.5">
            <span className="min-w-0 flex-1 truncate" title={muteScopeLabel(mute)}>
              {mute.check} · {muteScopeLabel(mute)}
            </span>
            <span className="min-w-0 flex-1 truncate text-fg-soft">{mute.reason}</span>
            <span className="shrink-0 text-fg-subtle">{mute.at}</span>
            <button
              type="button"
              aria-label={`Unmute ${muteKey(mute)}`}
              className="shrink-0 text-fg-soft hover:underline"
              onClick={() => {
                const startedOn = audit;
                unmuteFinding(mute)
                  .then((left) => {
                    if (!mounted.current || latestAudit.current !== startedOn) {
                      return;
                    }
                    setMutes(left);
                    onChanged();
                  })
                  .catch((err: unknown) => {
                    if (mounted.current && latestAudit.current === startedOn) {
                      setFailed(messageOf(err));
                    }
                  });
              }}
            >
              unmute
            </button>
          </li>
        ))}
      </ul>
    </details>
  );
}

function muteKey(mute: Mute): string {
  return [mute.check, mute.namespace ?? '', mute.ref ?? ''].join('|');
}

function muteScopeLabel(mute: Mute): string {
  if (mute.ref !== undefined && mute.ref !== '') {
    return mute.ref;
  }
  if (mute.namespace !== undefined && mute.namespace !== '') {
    return `everything in ${mute.namespace}`;
  }
  return 'everywhere';
}

function BaselineBar({ report, onChanged }: { report: CheckReportView; onChanged: () => void }) {
  const baseline = report.baseline;
  const onlyNew = useSettingsStore((state) => state.checksOnlyNew);
  const setOnlyNew = useSettingsStore((state) => state.setChecksOnlyNew);
  const showMuted = useSettingsStore((state) => state.checksShowMuted);
  const setShowMuted = useSettingsStore((state) => state.setChecksShowMuted);
  const [working, setWorking] = useState(false);
  const [failed, setFailed] = useState<string | null>(null);
  const scope = useContextScope();
  const operation = useScopedOperation();

  useEffect(() => {
    setWorking(false);
    setFailed(null);
  }, [scope]);

  function run(action: () => Promise<string>) {
    operation.current += 1;
    const token = operation.current;
    setWorking(true);
    setFailed(null);
    action()
      .then(() => {
        if (operation.current === token) {
          onChanged();
        }
      })
      .catch((err: unknown) => {
        if (operation.current === token) {
          setFailed(messageOf(err));
        }
      })
      .finally(() => {
        if (operation.current === token) {
          setWorking(false);
        }
      });
  }

  return (
    <div className="flex shrink-0 items-center gap-4 border-b border-edge px-3 py-1.5 text-fg-muted">
      <span>{baselineLabel(report)}</span>
      <button
        type="button"
        disabled={working}
        className="rounded border border-edge px-2 py-0.5 text-fg-strong disabled:text-fg-subtle"
        onClick={() => {
          run(takeBaseline);
        }}
      >
        {takeLabel(baseline, working)}
      </button>
      {baseline !== '' && (
        <>
          <button
            type="button"
            disabled={working}
            className="hover:underline disabled:text-fg-subtle"
            onClick={() => {
              run(clearBaseline);
            }}
          >
            forget it
          </button>
          <label className="flex items-center gap-1.5">
            <input
              type="checkbox"
              checked={onlyNew}
              onChange={(event) => {
                setOnlyNew(event.target.checked);
              }}
            />
            Only what is new
          </label>
        </>
      )}
      <label className="flex items-center gap-1.5">
        <input
          type="checkbox"
          checked={showMuted}
          onChange={(event) => {
            setShowMuted(event.target.checked);
          }}
        />
        Show what is muted
      </label>
      <BaselineFile baseline={baseline} onChanged={onChanged} onFailed={setFailed} />
      <ExportButton onFailed={setFailed} />
      {failed !== null && <span className="text-warn">{failed}</span>}
    </div>
  );
}

function BaselineFile({
  baseline,
  onChanged,
  onFailed,
}: {
  baseline: string;
  onChanged: () => void;
  onFailed: (message: string) => void;
}) {
  const pick = useRef<HTMLInputElement>(null);
  const operation = useScopedOperation();

  function save() {
    operation.current += 1;
    const token = operation.current;
    saveBaselineFile()
      .then((body) => {
        if (operation.current === token) {
          saveAs('spinoza-baseline.json', body);
        }
      })
      .catch((err: unknown) => {
        if (operation.current === token) {
          onFailed(messageOf(err));
        }
      });
  }

  async function load(file: File) {
    operation.current += 1;
    const token = operation.current;
    try {
      const body = await file.text();
      if (operation.current !== token) {
        return;
      }
      await loadBaselineFile(body);
      if (operation.current === token) {
        onChanged();
      }
    } catch (err: unknown) {
      if (operation.current === token) {
        onFailed(messageOf(err));
      }
    }
  }

  return (
    <>
      {baseline !== '' && (
        <button
          type="button"
          className="rounded border border-edge px-2 py-0.5 text-fg-soft"
          onClick={() => {
            save();
          }}
        >
          Save it to a file
        </button>
      )}
      <button
        type="button"
        className="rounded border border-edge px-2 py-0.5 text-fg-soft"
        onClick={() => {
          pick.current?.click();
        }}
      >
        Load one from a file
      </button>
      <input
        ref={pick}
        type="file"
        accept="application/json"
        aria-label="A baseline to load"
        className="hidden"
        onChange={(event) => {
          const file = event.target.files?.[0];
          event.target.value = '';
          if (file === undefined) {
            return;
          }
          void load(file);
        }}
      />
    </>
  );
}

function ExportButton({ onFailed }: { onFailed: (message: string) => void }) {
  const keep = useChecksFilter();
  const [working, setWorking] = useState(false);
  const scope = useContextScope();
  const operation = useScopedOperation();

  useEffect(() => {
    setWorking(false);
  }, [scope]);

  return (
    <button
      type="button"
      disabled={working}
      className="rounded border border-edge px-2 py-0.5 text-fg-soft disabled:text-fg-subtle"
      onClick={() => {
        operation.current += 1;
        const token = operation.current;
        setWorking(true);
        exportChecks(keep)
          .then((body) => {
            if (operation.current === token) {
              saveAs('spinoza-checks.csv', body);
            }
          })
          .catch((err: unknown) => {
            if (operation.current === token) {
              onFailed(messageOf(err));
            }
          })
          .finally(() => {
            if (operation.current === token) {
              setWorking(false);
            }
          });
      }}
    >
      {exportLabel(working)}
    </button>
  );
}

function exportLabel(working: boolean): string {
  if (working) {
    return 'Working';
  }
  return 'Export';
}

function Namespaces({ counts }: { counts: NamespaceCount[] }) {
  const picked = useSettingsStore((state) => state.checksNamespace);
  const setPicked = useSettingsStore((state) => state.setChecksNamespace);
  if (counts.length === 0) {
    return null;
  }

  return (
    <details className="shrink-0 border-b border-edge px-3 py-1.5 text-fg-muted">
      <summary className="cursor-pointer">{namespacesLabel(counts, picked)}</summary>
      <ul className="py-1">
        {counts.slice(0, NAMESPACES_SHOWN).map((entry) => (
          <li key={entry.namespace} className="flex items-baseline gap-3 py-0.5">
            <button
              type="button"
              className="min-w-0 flex-1 truncate text-left text-fg-strong hover:underline"
              onClick={() => {
                setPicked(pickedNext(picked, entry.namespace));
              }}
            >
              {entry.namespace}
            </button>
            {clustersLabel(entry.clusters) !== '' && (
              <span className="shrink-0 truncate text-fg-subtle">
                {clustersLabel(entry.clusters)}
              </span>
            )}
            <span className="w-16 shrink-0 text-right text-error">{entry.high}</span>
            <span className="w-16 shrink-0 text-right text-fg-soft">{entry.total}</span>
          </li>
        ))}
      </ul>
    </details>
  );
}

function AuditControls() {
  const floor = useSettingsStore((state) => state.checksMinSeverity);
  const setFloor = useSettingsStore((state) => state.setChecksMinSeverity);
  const wholeCluster = useSettingsStore((state) => state.checksWholeCluster);
  const setWholeCluster = useSettingsStore((state) => state.setChecksWholeCluster);
  const skipped = useSettingsStore((state) => state.checksSkipNamespaces);
  const setSkipped = useSettingsStore((state) => state.setChecksSkipNamespaces);
  const [typing, setTyping] = useState(skipped.join(','));
  const everyKind = useSettingsStore((state) => state.checksEveryKind);
  const setEveryKind = useSettingsStore((state) => state.setChecksEveryKind);
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
      <label
        className="flex items-center gap-1.5"
        title="Reads every kind the cluster reports, which is slow on a large one"
      >
        <input
          type="checkbox"
          checked={everyKind}
          onChange={(event) => {
            setEveryKind(event.target.checked);
          }}
        />
        Read every kind
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
  report,
  keep,
  fleet,
  onOpen,
  onChanged,
}: {
  category: CheckCategory;
  groups: CheckGroupView[];
  report: CheckReportView;
  keep: ChecksFilter;
  fleet: boolean;
  onOpen: (ref: ObjectRef, kind: string) => void;
  onChanged: () => void;
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
        <Group
          key={group.id}
          group={group}
          report={report}
          keep={keep}
          fleet={fleet}
          onOpen={onOpen}
          onChanged={onChanged}
        />
      ))}
    </section>
  );
}

export default function Checks({ onOpen }: ChecksProps) {
  const several = useTabStrip();
  const [fleet, setFleet] = useState(false);
  const showing = fleet && several;
  const { data, error, stale, reload } = useChecks(showing);
  const keep = useChecksFilter();
  const namespace = useSettingsStore((state) => state.checksNamespace);

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
      <BaselineBar report={data} onChanged={reload} />
      <MutesPanel audit={data} onChanged={reload} />
      <Namespaces counts={data.namespaces} />
      <YourRules />
      <div className="flex shrink-0 items-baseline gap-3 border-b border-edge px-3 py-1.5 text-fg-muted">
        <span className="min-w-0 flex-1">
          {scannedLabel(data.scanned, totalFindings(data), namespace)}
        </span>
        {several && (
          <label className="flex shrink-0 items-center gap-1.5 text-fg-soft">
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
        <span className="w-16 shrink-0 text-right">Severity</span>
        <span className="w-16 shrink-0 text-right">Findings</span>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto">
        {CATEGORY_ORDER.map((category) => (
          <Category
            key={category}
            category={category}
            groups={inCategory(data.groups, category)}
            report={data}
            keep={keep}
            fleet={showing}
            onOpen={onOpen}
            onChanged={reload}
          />
        ))}
      </div>
    </div>
  );
}

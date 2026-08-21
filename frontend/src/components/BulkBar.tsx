import { useEffect, useRef, useState } from 'react';
import type { BulkAccess, ObjectRef } from '../lib/types';
import { fetchBulkAccess } from '../lib/access';
import { deleteObject } from '../lib/object';
import { canRestart, runAction } from '../lib/objectActions';
import { notifyError, notifyOk } from '../store/toasts';
import { useContextList } from '../store/contexts';
import ConfirmByName from './ConfirmByName';

interface BulkBarProps {
  kind: string;
  targets: ObjectRef[];
  onDone: () => void;
  onClear: () => void;
}

type Pending = 'delete' | 'restart' | null;

// What the cluster said about the selection that is waiting to be confirmed.
interface Checked {
  refused: string[];
  total: number;
  reason: string;
}

const NAMES_SHOWN = 3;

function label(count: number, kind: string): string {
  if (count === 1) {
    return `1 ${kind} selected`;
  }
  return `${String(count)} ${kind} objects selected`;
}

function objects(count: number): string {
  if (count === 1) {
    return '1 object';
  }
  return `${String(count)} objects`;
}

function typedQuestion(count: number, cluster: string): string {
  return `Deleting ${objects(count)} on ${cluster} in one go — this asks for the cluster name, not an object name.`;
}

function verbFor(pending: Exclude<Pending, null>): string {
  if (pending === 'delete') {
    return 'Delete';
  }
  return 'Restart';
}

function outcome(done: number, failed: string[], verb: string): string {
  if (failed.length === 0) {
    return `${verb} ${String(done)}`;
  }
  return `${verb} ${String(done)}, ${String(failed.length)} failed: ${failed.join(', ')}`;
}

function within(at: number, total: number): boolean {
  if (at < 0) {
    return false;
  }
  return at < total;
}

function summarise(answer: BulkAccess, targets: ObjectRef[]): Checked {
  const refused: string[] = [];
  let reason = '';
  for (const row of answer.refused) {
    if (!within(row.at, targets.length)) {
      continue;
    }
    refused.push(targets[row.at].name);
    if (reason === '') {
      reason = row.reason;
    }
  }
  return { refused, total: targets.length, reason };
}

// A question the cluster would not answer stops nothing: the same rule the
// server follows for a check it could not put.
async function reviewOf(
  capability: Exclude<Pending, null>,
  refs: ObjectRef[],
): Promise<BulkAccess> {
  try {
    return await fetchBulkAccess(capability, refs);
  } catch {
    return { refused: [] };
  }
}

// A few rows are worth naming; more than that and the count says it better.
function some(checked: Checked): string {
  if (checked.refused.length > NAMES_SHOWN) {
    return `${String(checked.refused.length)} of ${String(checked.total)}`;
  }
  return checked.refused.join(', ');
}

function partialNote(checked: Checked | null): string {
  if (checked === null) {
    return '';
  }
  if (checked.refused.length === 0) {
    return '';
  }
  return ` ${some(checked)} will be refused: ${checked.reason}`;
}

// question is what the bar asks before it acts, once the cluster has had its
// say. Every row refused is not a question at all.
function question(pending: Exclude<Pending, null>, checked: Checked | null): string {
  if (checked === null) {
    return 'Checking what the cluster allows…';
  }
  if (checked.refused.length === checked.total) {
    return `The cluster refuses all ${String(checked.total)}: ${checked.reason}`;
  }
  return `${verbFor(pending)} ${objects(checked.total)}?${partialNote(checked)}`;
}

function refusesEverything(checked: Checked | null): boolean {
  if (checked === null) {
    return false;
  }
  return checked.refused.length === checked.total;
}

function selectionKey(targets: ObjectRef[]): string {
  return targets.map((ref) => `${ref.namespace}/${ref.name}`).join(',');
}

export default function BulkBar({ kind, targets, onDone, onClear }: BulkBarProps) {
  const [busy, setBusy] = useState(false);
  const [confirming, setConfirming] = useState<Pending>(null);
  const [checked, setChecked] = useState<Checked | null>(null);
  const asked = useRef(0);
  const list = useContextList();
  const protectedCluster = list.protection === 'protected';
  const key = selectionKey(targets);

  // A question that was put about one selection means nothing about the next.
  useEffect(() => {
    setConfirming(null);
    setChecked(null);
  }, [key]);

  async function check(capability: Exclude<Pending, null>, refs: ObjectRef[]) {
    asked.current += 1;
    const mine = asked.current;
    setChecked(null);
    const answer = await reviewOf(capability, refs);
    if (asked.current !== mine) {
      return;
    }
    setChecked(summarise(answer, refs));
  }

  function ask(pending: Exclude<Pending, null>) {
    setConfirming(pending);
    void check(pending, targets);
  }

  async function runAll(each: (ref: ObjectRef) => Promise<unknown>, verb: string) {
    setBusy(true);
    setConfirming(null);
    let done = 0;
    const failed: string[] = [];
    for (const ref of targets) {
      try {
        await each(ref);
        done += 1;
      } catch {
        failed.push(ref.name);
      }
    }
    setBusy(false);
    if (failed.length === 0) {
      notifyOk(outcome(done, failed, verb));
    } else {
      notifyError(outcome(done, failed, verb));
    }
    onDone();
  }

  function confirmDelete() {
    if (protectedCluster) {
      void runAll((ref) => deleteObject(ref, ref.name), 'Deleted');
      return;
    }
    void runAll((ref) => deleteObject(ref), 'Deleted');
  }

  function confirmRestart() {
    void runAll((ref) => runAction(ref, 'restart'), 'Restarted');
  }

  if (targets.length === 0) {
    return null;
  }

  const restartable = canRestart(targets[0]);
  const stopped = refusesEverything(checked);
  const answered = checked !== null;
  const partial = partialNote(checked);
  const typedGate = protectedCluster && confirming === 'delete' && answered && !stopped;

  return (
    <div
      role="status"
      className="flex shrink-0 items-center gap-2 border-b border-edge bg-surface-active px-2 py-1.5 text-xs"
    >
      <span className="text-fg-strong">{label(targets.length, kind)}</span>
      {confirming === null && (
        <>
          {restartable && (
            <button
              type="button"
              disabled={busy}
              onClick={() => {
                ask('restart');
              }}
              className="rounded border border-edge-strong px-2 py-0.5 text-fg hover:bg-surface-raised disabled:cursor-not-allowed disabled:text-fg-faint"
            >
              Restart
            </button>
          )}
          <button
            type="button"
            disabled={busy}
            onClick={() => {
              ask('delete');
            }}
            className="rounded border border-error-line px-2 py-0.5 text-error hover:bg-error-tint disabled:cursor-not-allowed disabled:text-fg-faint"
          >
            Delete
          </button>
        </>
      )}
      {typedGate && (
        <ConfirmByName
          open
          name={list.current.name}
          what={`${typedQuestion(targets.length, list.current.name)}${partial}`}
          onConfirm={confirmDelete}
          onCancel={() => {
            setConfirming(null);
          }}
        />
      )}
      {confirming !== null && !typedGate && (
        <>
          <span className="text-fg-muted">{question(confirming, checked)}</span>
          {answered && !stopped && (
            <button
              type="button"
              disabled={busy}
              onClick={() => {
                if (confirming === 'delete') {
                  confirmDelete();
                  return;
                }
                confirmRestart();
              }}
              className="rounded border border-error-line-strong bg-error-tint px-2 py-0.5 text-error-strong hover:bg-error-tint-strong disabled:cursor-not-allowed"
            >
              Confirm
            </button>
          )}
          <button
            type="button"
            disabled={busy}
            onClick={() => {
              setConfirming(null);
            }}
            className="rounded border border-edge px-2 py-0.5 text-fg-muted hover:bg-surface-raised disabled:cursor-not-allowed"
          >
            Cancel
          </button>
        </>
      )}
      <button
        type="button"
        onClick={onClear}
        className="ml-auto rounded border border-edge px-2 py-0.5 text-fg-muted hover:bg-surface-raised"
      >
        Clear selection
      </button>
    </div>
  );
}

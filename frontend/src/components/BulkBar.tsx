import { useState } from 'react';
import type { ObjectRef } from '../lib/types';
import { deleteObject } from '../lib/object';
import { canRestart, runAction } from '../lib/objectActions';
import { notifyError, notifyOk } from '../store/toasts';

interface BulkBarProps {
  kind: string;
  targets: ObjectRef[];
  onDone: () => void;
  onClear: () => void;
}

type Pending = 'delete' | 'restart' | null;

function label(count: number, kind: string): string {
  if (count === 1) {
    return `1 ${kind} selected`;
  }
  return `${String(count)} ${kind} objects selected`;
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

export default function BulkBar({ kind, targets, onDone, onClear }: BulkBarProps) {
  const [busy, setBusy] = useState(false);
  const [confirming, setConfirming] = useState<Pending>(null);

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
    void runAll((ref) => deleteObject(ref), 'Deleted');
  }

  function confirmRestart() {
    void runAll((ref) => runAction(ref, 'restart'), 'Restarted');
  }

  if (targets.length === 0) {
    return null;
  }

  const restartable = canRestart(targets[0]);

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
                setConfirming('restart');
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
              setConfirming('delete');
            }}
            className="rounded border border-error-line px-2 py-0.5 text-error hover:bg-error-tint disabled:cursor-not-allowed disabled:text-fg-faint"
          >
            Delete
          </button>
        </>
      )}
      {confirming !== null && (
        <>
          <span className="text-fg-muted">
            {verbFor(confirming)} {targets.length} object(s)?
          </span>
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

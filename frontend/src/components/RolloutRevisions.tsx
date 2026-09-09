import { Suspense, lazy, useCallback, useEffect, useState } from 'react';
import type { ObjectRef, Revision, RevisionDiff } from '../lib/types';
import { fetchRevisionDiff, fetchRevisions } from '../lib/rollout';
import { runAction } from '../lib/objectActions';
import { usePoll } from '../lib/usePoll';
import { notifyError, notifyOk } from '../store/toasts';
import { useProtectedCluster } from '../store/contexts';
import { useRefusal } from '../store/access';
import CapabilityState from './CapabilityState';
import ConfirmByName from './ConfirmByName';
import ActionGroup, { Action, ActionNote } from './ActionGroup';
import DisabledActionReasons from './DisabledActionReasons';
import { actionTitle, describedBy } from '../lib/actionAvailability';

const YamlDiff = lazy(() => import('./YamlDiff'));

const REVISIONS_POLL_MS = 15000;

interface RolloutRevisionsProps {
  target: ObjectRef;
  onDone: () => void;
}

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'that did not work';
}

function ageOf(revision: Revision): string {
  if (revision.createdAt === undefined || revision.createdAt === '') {
    return 'unknown';
  }
  return revision.createdAt;
}

export default function RolloutRevisions({ target, onDone }: RolloutRevisionsProps) {
  const load = useCallback(() => fetchRevisions(target), [target]);
  const {
    data: answer,
    error,
    reload,
  } = usePoll(load, {
    intervalMs: REVISIONS_POLL_MS,
    fallback: 'revisions request failed',
    resetKey: `${target.namespace}/${target.name}`,
  });

  const [against, setAgainst] = useState<number | null>(null);
  const [diff, setDiff] = useState<RevisionDiff | null>(null);
  const [diffError, setDiffError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [asking, setAsking] = useState<number | null>(null);
  const protectedCluster = useProtectedCluster();
  const noUndo = useRefusal(target, 'undo');

  const revisions = answer?.revisions ?? [];
  const current = revisions.find((one) => one.current === true) ?? null;

  useEffect(() => {
    setAgainst(null);
    setDiff(null);
    setDiffError(null);
    setAsking(null);
  }, [target.namespace, target.name]);

  const currentNumber = current?.number ?? null;

  useEffect(() => {
    if (against === null || currentNumber === null) {
      setDiff(null);
      return;
    }
    let live = true;
    setDiffError(null);
    fetchRevisionDiff(target, against, currentNumber)
      .then((got) => {
        if (live) {
          setDiff(got);
        }
      })
      .catch((err: unknown) => {
        if (live) {
          setDiff(null);
          setDiffError(errorMessage(err));
        }
      });
    return () => {
      live = false;
    };
  }, [against, currentNumber, target]);

  async function undo(revision: number, confirm?: string) {
    setBusy(true);
    try {
      const result = await runAction(target, 'undo', { revision, confirm });
      notifyOk(`${target.name}: ${result.message}`, target);
      setAgainst(null);
      reload();
      onDone();
    } catch (err: unknown) {
      notifyError(`undo ${target.name}: ${errorMessage(err)}`, target);
    } finally {
      setBusy(false);
    }
  }

  function askUndo(revision: number) {
    if (protectedCluster) {
      setAsking(revision);
      return;
    }
    void undo(revision);
  }

  if (answer === null) {
    if (error !== null) {
      return <div className="p-4 text-xs text-error">{error}</div>;
    }
    return <CapabilityState state="loading" what="revisions" />;
  }

  if (!answer.supported) {
    return (
      <CapabilityState
        state="empty"
        what="Revisions"
        why={answer.reason ?? 'this kind keeps no rollout history'}
      />
    );
  }

  if (revisions.length === 0) {
    return (
      <div className="flex min-h-0 flex-col">
        <div className="p-4 text-xs text-fg-muted">
          This workload has rolled out nothing spinoza can still read.
        </div>
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden text-xs">
      {error !== null && (
        <CapabilityState state="stale" what="Revisions" why={error} onRetry={reload} />
      )}
      <div className="min-h-0 shrink-0 overflow-y-auto">
        {revisions.map((revision) => (
          <article
            key={revision.name}
            className="flex flex-wrap items-baseline gap-2 border-b border-edge px-4 py-2"
          >
            <span className="font-mono text-fg-strong">#{revision.number}</span>
            {revision.current === true && <span className="text-ok">current</span>}
            <span className="text-fg-muted">{ageOf(revision)}</span>
            {revision.images !== undefined && (
              <span className="truncate font-mono text-fg-soft">{revision.images.join(' ')}</span>
            )}
            {revision.cause !== undefined && (
              <span className="truncate text-fg-muted">{revision.cause}</span>
            )}
            <span className="ml-auto">
              <ActionGroup>
                {revision.current !== true && (
                  <Action
                    label={against === revision.number ? 'Hide diff' : 'Diff'}
                    onClick={() => {
                      setAgainst(against === revision.number ? null : revision.number);
                    }}
                  />
                )}
                {revision.current !== true && (
                  <Action
                    label="Go back to this"
                    tone="caution"
                    onClick={() => {
                      askUndo(revision.number);
                    }}
                    disabled={busy || noUndo !== null}
                    describedBy={describedBy(noUndo, 'revisions-undo')}
                    title={actionTitle(noUndo)}
                  />
                )}
                {busy && <ActionNote>working</ActionNote>}
              </ActionGroup>
            </span>
          </article>
        ))}
      </div>
      <DisabledActionReasons
        reasons={[{ id: 'revisions-undo', label: 'Go back to this', reason: noUndo }]}
      />
      {diffError !== null && <div className="px-4 py-2 text-error">{diffError}</div>}
      {diff !== null && (
        <div className="flex min-h-0 flex-1 flex-col border-t border-edge">
          <div className="px-4 py-1.5 text-fg-muted">
            {diff.same
              ? `#${diff.from} and #${diff.to} hold the same pod template`
              : `#${diff.from} against #${diff.to}: ${diff.lines} lines differ`}
          </div>
          {!diff.same && (
            <div className="min-h-0 flex-1">
              <Suspense fallback={<CapabilityState state="loading" what="the diff" />}>
                <YamlDiff left={diff.left} right={diff.right} sideBySide />
              </Suspense>
            </div>
          )}
        </div>
      )}
      {asking !== null && (
        <ConfirmByName
          open
          name={target.name}
          what={`Put ${target.name} back to revision ${asking}`}
          onConfirm={() => {
            const revision = asking;
            setAsking(null);
            void undo(revision, target.name);
          }}
          onCancel={() => {
            setAsking(null);
          }}
        />
      )}
    </div>
  );
}

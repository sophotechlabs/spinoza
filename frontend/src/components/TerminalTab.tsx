import { useEffect, useState } from 'react';
import type { PodTarget } from '../lib/pods';
import { firstContainer, podRef } from '../lib/pods';
import { useLocalShellSupport } from '../lib/useLocalShell';
import { useActiveTerminal, useTerminalSessions, useTerminalsStore } from '../store/terminals';
import { useRefusal } from '../store/access';
import type { TerminalSession as Session } from '../store/terminals';
import TerminalSession, { LocalTerminalSession, NodeTerminalSession } from './TerminalSession';

interface TerminalTabProps {
  pod: PodTarget | null;
}

function tabClass(active: boolean): string {
  const base = 'flex items-center gap-1 rounded border px-1.5 py-0.5';
  if (active) {
    return `${base} border-edge-strong bg-surface-active text-fg-strong`;
  }
  return `${base} border-edge text-fg-muted hover:bg-surface-raised`;
}

function tabLabel(session: Session): string {
  if (session.kind === 'local') {
    return 'local';
  }
  if (session.kind === 'node') {
    return `node ${session.pod}`;
  }
  return `${session.pod}/${session.container}`;
}

function closeLabel(session: Session): string {
  if (session.kind === 'local') {
    return 'Close the local shell';
  }
  if (session.kind === 'node') {
    return `Close the shell on node ${session.pod}`;
  }
  return `Close the shell in ${session.pod}`;
}

export default function TerminalTab({ pod }: TerminalTabProps) {
  const sessions = useTerminalSessions();
  const active = useActiveTerminal();
  const open = useTerminalsStore((state) => state.open);
  const openLocal = useTerminalsStore((state) => state.openLocal);
  const focus = useTerminalsStore((state) => state.focus);
  const close = useTerminalsStore((state) => state.close);
  const [container, setContainer] = useState(() => firstContainer(pod));
  const support = useLocalShellSupport();
  const noExec = useRefusal(podRef(pod), 'exec');

  const podKey = pod === null ? '' : `${pod.namespace}/${pod.name}`;
  const [lastPodKey, setLastPodKey] = useState(podKey);
  if (podKey !== lastPodKey) {
    setLastPodKey(podKey);
    setContainer(firstContainer(pod));
  }

  const local = support?.available === true;
  const [greeted, setGreeted] = useState(false);

  useEffect(() => {
    if (!local || greeted) {
      return;
    }
    setGreeted(true);
    openLocal();
  }, [local, greeted, openLocal]);

  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <div className="flex shrink-0 flex-wrap items-center gap-1.5 border-b border-edge px-2 py-1.5 text-[11px]">
        {sessions.map((session) => (
          <span key={session.id} className={tabClass(session.id === active)}>
            <button
              type="button"
              aria-pressed={session.id === active}
              onClick={() => {
                focus(session.id);
              }}
              className="max-w-52 cursor-pointer truncate"
            >
              {tabLabel(session)}
            </button>
            <button
              type="button"
              aria-label={closeLabel(session)}
              onClick={() => {
                close(session.id);
              }}
              className="cursor-pointer text-fg-muted hover:text-fg-strong"
            >
              ×
            </button>
          </span>
        ))}
        <span className="ml-auto flex items-center gap-1.5">
          {local && (
            <button
              type="button"
              onClick={openLocal}
              className="rounded border border-edge-strong px-1.5 py-0.5 text-fg hover:bg-surface-active"
            >
              Local shell
            </button>
          )}
          {pod !== null && container !== '' && pod.containers.length > 1 && (
            <select
              aria-label="Container"
              value={container}
              onChange={(event) => {
                setContainer(event.target.value);
              }}
              className="rounded border border-edge-strong bg-surface-raised px-1 py-0.5 text-fg"
            >
              {pod.containers.map((name) => (
                <option key={name} value={name}>
                  {name}
                </option>
              ))}
            </select>
          )}
          {pod !== null && container !== '' && (
            <button
              type="button"
              onClick={() => {
                open(pod.namespace, pod.name, container);
              }}
              disabled={noExec !== null}
              title={noExec ?? undefined}
              className="rounded border border-edge-strong px-1.5 py-0.5 text-fg hover:bg-surface-active disabled:cursor-not-allowed disabled:text-fg-faint"
            >
              Shell in {pod.name}
            </button>
          )}
        </span>
      </div>
      {sessions.length === 0 && (
        <div className="p-3 text-[11px] text-fg-muted">
          {support?.available === false && <p>{support.reason}</p>}
          <p>No shells open. Pick a pod and open one above.</p>
        </div>
      )}
      {sessions.map((session) => (
        <div
          key={session.id}
          hidden={session.id !== active}
          className={session.id === active ? 'flex min-h-0 min-w-0 flex-1 flex-col' : 'hidden'}
        >
          {session.kind === 'local' && <LocalTerminalSession />}
          {session.kind === 'node' && <NodeTerminalSession node={session.pod} />}
          {session.kind === 'pod' && (
            <TerminalSession
              namespace={session.namespace}
              pod={session.pod}
              container={session.container}
            />
          )}
        </div>
      ))}
    </div>
  );
}

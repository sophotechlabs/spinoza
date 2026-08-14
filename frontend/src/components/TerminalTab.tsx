import { useState } from 'react';
import type { PodTarget } from '../lib/pods';
import { firstContainer } from '../lib/pods';
import { useTerminalsStore } from '../store/terminals';
import TerminalSession from './TerminalSession';

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

export default function TerminalTab({ pod }: TerminalTabProps) {
  const sessions = useTerminalsStore((state) => state.sessions);
  const active = useTerminalsStore((state) => state.active);
  const open = useTerminalsStore((state) => state.open);
  const focus = useTerminalsStore((state) => state.focus);
  const close = useTerminalsStore((state) => state.close);
  const [container, setContainer] = useState(() => firstContainer(pod));

  const podKey = pod === null ? '' : `${pod.namespace}/${pod.name}`;
  const [lastPodKey, setLastPodKey] = useState(podKey);
  if (podKey !== lastPodKey) {
    setLastPodKey(podKey);
    setContainer(firstContainer(pod));
  }

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
              {session.pod}/{session.container}
            </button>
            <button
              type="button"
              aria-label={`Close the shell in ${session.pod}`}
              onClick={() => {
                close(session.id);
              }}
              className="cursor-pointer text-fg-muted hover:text-fg-strong"
            >
              ×
            </button>
          </span>
        ))}
        {pod !== null && container !== '' && (
          <span className="ml-auto flex items-center gap-1.5">
            {pod.containers.length > 1 && (
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
            <button
              type="button"
              onClick={() => {
                open(pod.namespace, pod.name, container);
              }}
              className="rounded border border-edge-strong px-1.5 py-0.5 text-fg hover:bg-surface-active"
            >
              Shell in {pod.name}
            </button>
          </span>
        )}
      </div>
      {sessions.length === 0 && (
        <div className="p-3 text-[11px] text-fg-muted">
          No shells open. Select a pod and open one from the button above.
        </div>
      )}
      {sessions.map((session) => (
        <div
          key={session.id}
          hidden={session.id !== active}
          className={session.id === active ? 'flex min-h-0 min-w-0 flex-1 flex-col' : 'hidden'}
        >
          <TerminalSession
            namespace={session.namespace}
            pod={session.pod}
            container={session.container}
          />
        </div>
      ))}
    </div>
  );
}

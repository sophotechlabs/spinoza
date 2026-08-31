import { useEffect, useState } from 'react';
import type { ObjectPort, ObjectRef, PortForward } from '../lib/types';
import { refreshForwards, startForward, stopForward } from '../lib/portForward';
import { forwardURL, openExternal } from '../lib/openExternal';
import { notifyError, notifyOk } from '../store/toasts';
import { useForwards } from '../store/forwards';
import { useClusterMode } from '../store/identity';
import { FORWARDS_ARE_LOCAL } from '../lib/portForward';
import { useRefusal } from '../store/access';
import Announce from './Announce';

interface InspectPortsProps {
  target: ObjectRef;
  kind: string;
  ports: ObjectPort[];
}

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'port forward failed';
}

function portLabel(port: ObjectPort): string {
  if (port.name === undefined || port.name === '') {
    return String(port.port);
  }
  return `${port.port} ${port.name}`;
}

function forwardFor(
  forwards: PortForward[],
  kind: string,
  target: ObjectRef,
  port: number,
): PortForward | null {
  for (const forward of forwards) {
    if (forward.kind !== kind) {
      continue;
    }
    if (forward.namespace !== target.namespace || forward.name !== target.name) {
      continue;
    }
    if (forward.remotePort === port) {
      return forward;
    }
  }
  return null;
}

export default function InspectPorts({ target, kind, ports }: InspectPortsProps) {
  const forwards = useForwards();
  const [busy, setBusy] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const served = useClusterMode();
  const noForward = useRefusal(target, 'portForward');

  useEffect(() => {
    if (served) {
      return;
    }
    void refreshForwards();
  }, [served, target.namespace, target.name]);

  async function forward(port: number) {
    setBusy(port);
    setError(null);
    try {
      const started = await startForward(kind, target, port);
      notifyOk(`Forwarding ${target.name} 127.0.0.1:${started.localPort} to ${port}`, target);
      await refreshForwards();
    } catch (err: unknown) {
      const message = errorMessage(err);
      setError(message);
      notifyError(`Forwarding ${target.name} port ${port}: ${message}`, target);
    } finally {
      setBusy(null);
    }
  }

  async function stop(running: PortForward) {
    setBusy(running.remotePort);
    setError(null);
    try {
      await stopForward(running.id);
      notifyOk(`Stopped forwarding ${target.name} port ${running.remotePort}`, target);
      await refreshForwards();
    } catch (err: unknown) {
      const message = errorMessage(err);
      setError(message);
      notifyError(`Stopping the forward: ${message}`);
    } finally {
      setBusy(null);
    }
  }

  return (
    <section className="border-b border-edge px-4 py-3">
      <h3 className="mb-2 text-[11px] font-semibold tracking-wide text-fg-muted uppercase">
        Ports
      </h3>
      <div className="flex flex-col gap-1">
        {ports.map((port) => {
          const running = forwardFor(forwards, kind, target, port.port);
          return (
            <div key={`${port.port}-${port.name ?? ''}`} className="flex items-center gap-2">
              <span className="text-fg">{portLabel(port)}</span>
              {served && <span className="ml-auto text-fg-muted">{FORWARDS_ARE_LOCAL}</span>}
              {!served && running === null ? (
                <button
                  type="button"
                  onClick={() => void forward(port.port)}
                  disabled={busy !== null || noForward !== null}
                  title={noForward ?? undefined}
                  className="ml-auto rounded border border-edge-strong px-1.5 py-0.5 text-fg-soft hover:bg-surface-active disabled:cursor-not-allowed disabled:text-fg-faint"
                >
                  Forward
                </button>
              ) : null}
              {!served && running !== null ? (
                <>
                  <span className="ml-auto text-ok">
                    127.0.0.1:{running.localPort} to {port.port}
                  </span>
                  <button
                    type="button"
                    onClick={() => {
                      openExternal(forwardURL(running.localPort));
                    }}
                    title={`Open ${forwardURL(running.localPort)}`}
                    aria-label={`Open 127.0.0.1:${running.localPort} in a browser`}
                    className="rounded border border-edge-strong px-1.5 py-0.5 text-fg-soft hover:bg-surface-active"
                  >
                    Open ↗
                  </button>
                  <button
                    type="button"
                    onClick={() => void stop(running)}
                    disabled={busy !== null}
                    aria-label={`Stop forwarding port ${port.port}`}
                    className="rounded border border-edge-strong px-1.5 py-0.5 text-fg-soft hover:bg-surface-active disabled:cursor-not-allowed disabled:text-fg-faint"
                  >
                    Stop
                  </button>
                </>
              ) : null}
            </div>
          );
        })}
      </div>
      <Announce message={error} urgent className="mt-1.5 break-words text-error" />
    </section>
  );
}

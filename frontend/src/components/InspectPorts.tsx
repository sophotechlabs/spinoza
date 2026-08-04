import { useState } from 'react';
import type { ObjectPort, ObjectRef } from '../lib/types';
import { refreshForwards, startForward } from '../lib/portForward';
import { notifyError, notifyOk } from '../store/toasts';
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
  return `${port.port} · ${port.name}`;
}

export default function InspectPorts({ target, kind, ports }: InspectPortsProps) {
  const [busy, setBusy] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  async function forward(port: number) {
    setBusy(port);
    setError(null);
    setNotice(null);
    try {
      const started = await startForward(kind, target, port);
      const route = `127.0.0.1:${started.localPort} → ${port}`;
      setNotice(route);
      notifyOk(`Forwarding ${target.name} ${route}`);
      await refreshForwards();
    } catch (err: unknown) {
      const message = errorMessage(err);
      setError(message);
      notifyError(`Forwarding ${target.name} port ${port}: ${message}`);
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
        {ports.map((port) => (
          <div key={`${port.port}-${port.name ?? ''}`} className="flex items-center gap-2">
            <span className="text-fg">{portLabel(port)}</span>
            <button
              type="button"
              onClick={() => void forward(port.port)}
              disabled={busy !== null}
              className="ml-auto rounded border border-edge-strong px-1.5 py-0.5 text-fg-soft hover:bg-surface-active disabled:cursor-not-allowed disabled:text-fg-faint"
            >
              Forward
            </button>
          </div>
        ))}
      </div>
      <Announce message={error} urgent className="mt-1.5 break-words text-error" />
      <Announce message={notice} className="mt-1.5 text-ok" />
    </section>
  );
}

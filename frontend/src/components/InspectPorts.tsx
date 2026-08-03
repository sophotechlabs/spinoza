import { useState } from 'react';
import type { ObjectPort, ObjectRef } from '../lib/types';
import { refreshForwards, startForward } from '../lib/portForward';

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
      setNotice(`127.0.0.1:${started.localPort} → ${port}`);
      await refreshForwards();
    } catch (err: unknown) {
      setError(errorMessage(err));
    } finally {
      setBusy(null);
    }
  }

  return (
    <section className="border-b border-neutral-800 px-4 py-3">
      <h3 className="mb-2 text-[11px] font-semibold tracking-wide text-neutral-400 uppercase">
        Ports
      </h3>
      <div className="flex flex-col gap-1">
        {ports.map((port) => (
          <div key={`${port.port}-${port.name ?? ''}`} className="flex items-center gap-2">
            <span className="text-neutral-200">{portLabel(port)}</span>
            <button
              type="button"
              onClick={() => void forward(port.port)}
              disabled={busy !== null}
              className="ml-auto rounded border border-neutral-700 px-1.5 py-0.5 text-neutral-300 hover:bg-neutral-800 disabled:cursor-not-allowed disabled:text-neutral-600"
            >
              Forward
            </button>
          </div>
        ))}
      </div>
      {error !== null && <p className="mt-1.5 break-words text-red-400">{error}</p>}
      {notice !== null && <p className="mt-1.5 text-green-400">{notice}</p>}
    </section>
  );
}

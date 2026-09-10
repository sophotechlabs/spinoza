import { useCallback, useEffect, useRef, useState } from 'react';
import { fetchTranscript, fetchTranscripts, sizeText } from '../lib/transcripts';
import { usePoll } from '../lib/usePoll';
import CapabilityState from './CapabilityState';
import { CONTROL } from '../lib/controls';
import { reasonOf } from '../lib/object';

const SESSIONS_POLL_MS = 30000;

export default function RecordedSessions() {
  const load = useCallback(() => fetchTranscripts(), []);
  const { data: page, error, reload } = usePoll(load, { intervalMs: SESSIONS_POLL_MS });
  const [open, setOpen] = useState<string | null>(null);
  const [text, setText] = useState<string | null>(null);
  const [failed, setFailed] = useState<string | null>(null);

  const reading = useRef(0);

  useEffect(() => {
    return () => {
      reading.current += 1;
    };
  }, []);

  function show(id: string) {
    reading.current += 1;
    const token = reading.current;
    if (open === id) {
      setOpen(null);
      setText(null);
      return;
    }
    setOpen(id);
    setText(null);
    setFailed(null);
    fetchTranscript(id)
      .then((body) => {
        if (reading.current === token) {
          setText(body);
        }
      })
      .catch((err: unknown) => {
        if (reading.current === token) {
          setFailed(reasonOf(err, 'that session could not be read'));
        }
      });
  }

  if (page === null) {
    if (error !== null) {
      return <CapabilityState state="failed" what="Recorded sessions" why={error} />;
    }
    return <CapabilityState state="loading" what="recorded sessions" />;
  }

  if (!page.recording) {
    return (
      <CapabilityState
        state="empty"
        what="Recorded sessions"
        why={page.reason ?? 'this deployment does not record terminal sessions'}
      />
    );
  }

  return (
    <section className="flex min-h-0 flex-col text-xs">
      {error !== null && (
        <CapabilityState state="stale" what="Recorded sessions" why={error} onRetry={reload} />
      )}
      {page.sessions.length === 0 && (
        <p className="p-3 text-fg-muted">
          No terminal has been opened since recording was turned on.
        </p>
      )}
      <ul className="min-h-0 overflow-y-auto">
        {page.sessions.map((one) => (
          <li key={one.id} className="border-b border-edge px-3 py-1">
            <div className="flex flex-wrap items-baseline gap-2">
              <span className="text-fg-muted">{one.at}</span>
              <span className="text-fg-strong">{one.actor}</span>
              <span className="text-fg-soft">{one.kind}</span>
              <span className="truncate font-mono text-fg-soft">{one.target}</span>
              <span className="text-fg-muted">{sizeText(one.bytes)}</span>
              <button
                type="button"
                onClick={() => {
                  show(one.id);
                }}
                className={`${CONTROL} ml-auto border-edge text-fg-soft hover:bg-surface-raised`}
              >
                {open === one.id ? 'Hide' : 'Read'}
              </button>
            </div>
            {open === one.id && failed !== null && <p className="mt-1 text-error">{failed}</p>}
            {open === one.id && failed === null && text === null && (
              <p className="mt-1 text-fg-muted">reading it…</p>
            )}
            {open === one.id && text !== null && (
              <pre className="mt-1 max-h-72 overflow-auto rounded border border-edge bg-surface p-2 font-mono text-fg-soft">
                {text}
              </pre>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}

import { useEffect, useState } from 'react';
import type { K8sEvent } from '../lib/types';
import { fetchEvents } from '../lib/object';

const EVENTS_POLL_MS = 10000;

interface InspectEventsProps {
  namespace: string;
  uid: string;
}

function eventColor(type: string): string {
  if (type === 'Warning') {
    return 'text-amber-400';
  }
  return 'text-neutral-400';
}

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'events request failed';
}

export default function InspectEvents({ namespace, uid }: InspectEventsProps) {
  const [events, setEvents] = useState<K8sEvent[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let mounted = true;
    let inFlight = false;
    const load = async () => {
      if (inFlight) {
        return;
      }
      inFlight = true;
      try {
        const data = await fetchEvents(namespace, uid);
        if (mounted) {
          setEvents(data);
          setError(null);
        }
      } catch (err: unknown) {
        if (mounted) {
          setError(errorMessage(err));
        }
      } finally {
        inFlight = false;
      }
    };
    void load();
    const timer = setInterval(() => {
      void load();
    }, EVENTS_POLL_MS);
    return () => {
      mounted = false;
      clearInterval(timer);
    };
  }, [namespace, uid]);

  if (error !== null) {
    return <div className="p-4 text-xs text-red-400">{error}</div>;
  }

  if (events === null) {
    return <div className="p-4 text-xs text-neutral-600">Loading events…</div>;
  }

  if (events.length === 0) {
    return <div className="p-4 text-xs text-neutral-600">No events for this object.</div>;
  }

  return (
    <div className="overflow-y-auto text-xs">
      {events.map((event, index) => (
        <article
          key={`${event.reason}-${event.lastSeen}-${index}`}
          className="border-b border-neutral-900 px-4 py-2"
        >
          <div className="flex items-baseline gap-2">
            <span className={eventColor(event.type)}>{event.reason}</span>
            <span className="text-neutral-600">{event.source}</span>
            <span className="ml-auto shrink-0 text-neutral-600">{event.lastSeen}</span>
          </div>
          <p className="mt-0.5 break-words text-neutral-300">{event.message}</p>
          {event.count > 1 && (
            <p className="mt-0.5 text-[11px] text-neutral-600">seen {event.count} times</p>
          )}
        </article>
      ))}
    </div>
  );
}

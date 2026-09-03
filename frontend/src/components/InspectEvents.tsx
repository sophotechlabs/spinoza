import { useCallback } from 'react';
import type { ReactNode } from 'react';
import { fetchEvents } from '../lib/object';
import { usePoll } from '../lib/usePoll';
import StaleBanner from './StaleBanner';
import Loading from './Loading';

const EVENTS_POLL_MS = 10000;

interface InspectEventsProps {
  namespace: string;
  uid: string;
  active?: boolean;
}

function eventColor(type: string): string {
  if (type === 'Warning') {
    return 'text-warn';
  }
  return 'text-fg-muted';
}

export default function InspectEvents({ namespace, uid, active = true }: InspectEventsProps) {
  const load = useCallback(() => fetchEvents(namespace, uid), [namespace, uid]);
  const {
    data: events,
    error,
    reload,
  } = usePoll(load, {
    intervalMs: EVENTS_POLL_MS,
    enabled: active,
    fallback: 'events request failed',
    resetKey: `${namespace}/${uid}`,
  });

  if (events === null) {
    if (error !== null) {
      return <div className="p-4 text-xs text-error">{error}</div>;
    }
    return <Loading what="events" />;
  }

  let notice: ReactNode = null;
  if (error !== null) {
    notice = <StaleBanner what="Events" message={error} onRetry={reload} />;
  }

  if (events.length === 0) {
    return (
      <div className="flex min-h-0 flex-col">
        {notice}
        <div className="p-4 text-xs text-fg-muted">No events for this object.</div>
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-col overflow-y-auto text-xs">
      {notice}
      {events.map((event, index) => (
        <article
          key={`${event.reason}-${event.lastSeen}-${index}`}
          className="border-b border-edge px-4 py-2"
        >
          <div className="flex items-baseline gap-2">
            <span className={eventColor(event.type)}>{event.reason}</span>
            <span className="text-fg-muted">{event.source}</span>
            <span className="ml-auto shrink-0 text-fg-muted">{event.lastSeen}</span>
          </div>
          <p className="mt-0.5 break-words text-fg-soft">{event.message}</p>
          {event.count > 1 && (
            <p className="mt-0.5 text-[11px] text-fg-muted">seen {event.count} times</p>
          )}
        </article>
      ))}
    </div>
  );
}

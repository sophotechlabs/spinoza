import { useState } from 'react';
import type { Health } from '../store/clusterHealth';
import { causePhrase, quietFor } from '../lib/health';
import { useNow } from '../lib/useNow';

interface ClusterBannerProps {
  cluster: string;
  health: Health;
  onReconnect: () => void;
}

const TICK_MS = 1000;

function headline(cluster: string, health: Health, now: number): string {
  const quiet = quietFor(health.since, now);
  if (!health.reachable) {
    if (quiet === '') {
      return `${cluster} is not answering.`;
    }
    return `${cluster} stopped answering ${quiet} ago.`;
  }
  return `${cluster} missed a ping.`;
}

function detail(health: Health): string {
  const phrase = causePhrase(health.cause);
  if (!health.reachable) {
    if (phrase === '') {
      return 'What follows is the last it sent.';
    }
    return `What follows is the last it sent — ${phrase}.`;
  }
  if (phrase === '') {
    return 'Still showing what it last sent.';
  }
  return `Still showing what it last sent — ${phrase}.`;
}

function tone(reachable: boolean): string {
  if (reachable) {
    return 'border-warn-line bg-warn-tint/40 text-warn-strong';
  }
  return 'border-error-line bg-error-tint/40 text-error-strong';
}

function nameTone(reachable: boolean): string {
  if (reachable) {
    return 'text-warn';
  }
  return 'text-error';
}

function buttonTone(reachable: boolean): string {
  if (reachable) {
    return 'border-warn-line-strong text-warn-strong hover:bg-warn-tint';
  }
  return 'border-error-line-strong text-error-contrast hover:bg-error-tint-strong';
}

export default function ClusterBanner({ cluster, health, onReconnect }: ClusterBannerProps) {
  const now = useNow(TICK_MS);
  const [open, setOpen] = useState(false);

  if (health.reachable && !health.wobbling) {
    return null;
  }

  let raw = null;
  if (open && health.reason !== '') {
    raw = <span className="min-w-0 basis-full break-words opacity-80">{health.reason}</span>;
  }

  let details = null;
  if (health.reason !== '') {
    details = (
      <button
        type="button"
        aria-expanded={open}
        onClick={() => {
          setOpen(!open);
        }}
        className="shrink-0 cursor-pointer underline underline-offset-2"
      >
        Details
      </button>
    );
  }

  return (
    <div
      role="status"
      aria-label="The cluster stopped answering"
      className={`flex shrink-0 flex-wrap items-baseline gap-2 border-b px-3 py-1.5 text-xs ${tone(health.reachable)}`}
    >
      <span className={`shrink-0 font-semibold ${nameTone(health.reachable)}`}>
        {headline(cluster, health, now)}
      </span>
      <span className="min-w-0 flex-1 truncate">{detail(health)}</span>
      {details}
      <button
        type="button"
        onClick={onReconnect}
        className={`shrink-0 rounded border px-1.5 py-0.5 ${buttonTone(health.reachable)}`}
      >
        Reconnect now
      </button>
      {raw}
    </div>
  );
}

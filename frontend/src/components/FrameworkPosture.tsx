import { useCallback, useState } from 'react';
import type { FrameworkControl } from '../lib/types';
import { fetchPosture } from '../lib/checks';
import { usePoll } from '../lib/usePoll';
import CapabilityState from './CapabilityState';
import { useChecksInterval } from '../store/settings';

const SCOPE_LABELS: Record<string, string> = {
  covered: 'checked',
  uncovered: 'no check answers this',
  'out of scope': 'nothing that reads a live cluster can answer this',
};

function scopeClass(control: FrameworkControl): string {
  if (control.scope !== 'covered') {
    return 'text-fg-subtle';
  }
  if (control.failing > 0) {
    return 'text-warn';
  }
  return 'text-ok';
}

function verdict(control: FrameworkControl): string {
  if (control.scope !== 'covered') {
    return SCOPE_LABELS[control.scope] ?? control.scope;
  }
  if (control.failing === 0) {
    return 'nothing fails this';
  }
  const objects = control.objects ?? 0;
  if (objects > 0) {
    return `${String(control.failing)} findings on ${String(objects)} objects`;
  }
  return `${String(control.failing)} findings`;
}

export default function FrameworkPosture() {
  const [framework, setFramework] = useState('');
  const seconds = useChecksInterval();
  const load = useCallback(() => fetchPosture(framework), [framework]);
  const {
    data: posture,
    error,
    reload,
  } = usePoll(load, {
    intervalMs: seconds * 1000,
    fallback: 'framework posture failed',
    resetKey: framework,
  });

  if (posture === null) {
    if (error !== null) {
      return <CapabilityState state="failed" what="Framework posture" why={error} />;
    }
    return <CapabilityState state="loading" what="framework posture" />;
  }

  const covered = posture.controls.filter((one) => one.scope === 'covered');
  const failing = covered.filter((one) => one.failing > 0);

  return (
    <section className="flex min-h-0 flex-col text-xs">
      {error !== null && (
        <CapabilityState state="stale" what="Framework posture" why={error} onRetry={reload} />
      )}
      {posture.reason !== undefined && posture.reason !== '' && (
        <CapabilityState state="partial" why={posture.reason} />
      )}
      <div className="flex flex-wrap items-center gap-2 border-b border-edge px-3 py-1.5">
        <label className="text-fg-muted" htmlFor="posture-framework">
          Framework
        </label>
        <select
          id="posture-framework"
          value={framework}
          onChange={(event) => {
            setFramework(event.target.value);
          }}
          className="rounded border border-edge bg-surface px-1 py-0.5 text-fg-soft"
        >
          <option value="">every framework</option>
          {posture.frameworks.map((one) => (
            <option key={one} value={one}>
              {one}
            </option>
          ))}
        </select>
        <span className="text-fg-muted">
          {failing.length} of {covered.length} checked controls have something failing
        </span>
      </div>
      <div className="min-h-0 flex-1 overflow-auto">
        {posture.controls.length === 0 && (
          <p className="p-3 text-fg-muted">No controls are cataloged for that framework.</p>
        )}
        <ul>
          {posture.controls.map((control) => (
            <li
              key={`${control.framework}/${control.control}`}
              className="flex flex-wrap items-baseline gap-2 border-b border-edge px-3 py-1"
            >
              <span className="w-16 shrink-0 font-mono text-fg-strong">{control.control}</span>
              <span className="min-w-0 flex-1 truncate text-fg-soft">{control.title}</span>
              <span className={scopeClass(control)}>{verdict(control)}</span>
              {(control.muted ?? 0) > 0 && (
                <span className="text-fg-muted">{control.muted} muted</span>
              )}
              {control.reason !== undefined && control.reason !== '' && (
                <span className="w-full text-fg-subtle">{control.reason}</span>
              )}
            </li>
          ))}
        </ul>
      </div>
    </section>
  );
}

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

function mutedText(control: FrameworkControl): string {
  const muted = control.muted ?? 0;
  if (muted === 0) {
    return '';
  }
  return `${String(muted)} muted`;
}

function ControlRow({ control, every }: { control: FrameworkControl; every: boolean }) {
  return (
    <tr className="border-b border-edge align-baseline">
      {every && (
        <td title={control.framework} className="truncate px-3 py-1 text-fg-muted">
          {control.framework}
        </td>
      )}
      <td title={control.control} className="truncate px-3 py-1 font-mono text-fg-strong">
        {control.control}
      </td>
      <td title={control.title} className="truncate px-3 py-1 text-fg-soft">
        {control.title}
      </td>
      <td className="px-3 py-1">
        <span className={scopeClass(control)}>{verdict(control)}</span>
        {mutedText(control) !== '' && (
          <span className="ml-2 text-fg-muted">{mutedText(control)}</span>
        )}
        {control.reason !== undefined && control.reason !== '' && (
          <span className="block text-fg-subtle">{control.reason}</span>
        )}
      </td>
    </tr>
  );
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
  const every = framework === '';

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
        {posture.controls.length > 0 && (
          <table className="w-full table-fixed">
            <thead className="sticky top-0 bg-surface text-left text-fg-muted">
              <tr className="border-b border-edge">
                {every && (
                  <th scope="col" className="w-48 px-3 py-1 font-normal">
                    Framework
                  </th>
                )}
                <th scope="col" className="w-56 px-3 py-1 font-normal">
                  Control
                </th>
                <th scope="col" className="px-3 py-1 font-normal">
                  Title
                </th>
                <th scope="col" className="w-80 px-3 py-1 font-normal">
                  Result
                </th>
              </tr>
            </thead>
            <tbody>
              {posture.controls.map((control) => (
                <ControlRow
                  key={`${control.framework}/${control.control}`}
                  control={control}
                  every={every}
                />
              ))}
            </tbody>
          </table>
        )}
      </div>
    </section>
  );
}

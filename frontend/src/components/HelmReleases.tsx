import { useState } from 'react';
import type { ReactNode } from 'react';
import type { HelmRelease, ObjectRef } from '../lib/types';
import {
  latestColor,
  latestLabel,
  latestNote,
  statusDot,
  statusLabel,
  statusText,
  useHelmReleases,
} from '../lib/helm';
import { ago } from '../lib/time';
import { useNow } from '../lib/useNow';
import LoadFailure from './LoadFailure';
import LoadWarning from './LoadWarning';
import StaleBanner from './StaleBanner';
import HelmReleaseDetail from './HelmReleaseDetail';

interface HelmReleasesProps {
  active?: boolean;
  onSelectResource: (ref: ObjectRef) => void;
}

function releaseKey(release: HelmRelease): string {
  return `${release.namespace}/${release.name}`;
}

function sameRelease(left: HelmRelease | null, right: HelmRelease): boolean {
  if (left === null) {
    return false;
  }
  return releaseKey(left) === releaseKey(right);
}

function rowClass(selected: boolean): string {
  const base = 'border-b border-edge';
  if (selected) {
    return `${base} bg-surface-active`;
  }
  return `${base} hover:bg-surface-raised`;
}

function orDash(value: string): string {
  if (value === '') {
    return '—';
  }
  return value;
}

function matching(releases: HelmRelease[], query: string): HelmRelease[] {
  const needle = query.trim().toLowerCase();
  if (needle === '') {
    return releases;
  }
  return releases.filter((release) => {
    const haystack = `${release.name} ${release.namespace} ${release.chart}`.toLowerCase();
    return haystack.includes(needle);
  });
}

export default function HelmReleases({ active = true, onSelectResource }: HelmReleasesProps) {
  const { data, error, reload } = useHelmReleases(active);
  const [query, setQuery] = useState('');
  const [selected, setSelected] = useState<HelmRelease | null>(null);
  const now = useNow();

  if (data === null) {
    if (error !== null) {
      return <LoadFailure what="Helm releases" message={error} />;
    }
    return (
      <div className="flex h-full items-center justify-center text-xs text-fg-muted">
        Loading Helm releases…
      </div>
    );
  }

  let notice: ReactNode = null;
  if (error !== null) {
    notice = <StaleBanner what="Helm releases" message={error} onRetry={reload} />;
  }

  const visible = matching(data.releases, query);

  return (
    <div className="flex h-full min-h-0 flex-col text-xs">
      {notice}
      {data.error !== undefined && <LoadWarning message={data.error} />}
      <div className="flex shrink-0 items-center gap-2 border-b border-edge px-3 py-1.5">
        <input
          type="search"
          aria-label="Filter releases"
          placeholder="Filter…"
          value={query}
          onChange={(event) => {
            setQuery(event.target.value);
          }}
          className="w-56 rounded border border-edge bg-surface-raised px-2 py-0.5 text-fg placeholder:text-fg-muted focus:border-edge-emphasis"
        />
        <span className="text-fg-muted">
          {visible.length} of {data.releases.length}
        </span>
      </div>
      {data.releases.length === 0 && (
        <div className="flex flex-1 items-center justify-center text-fg-muted">
          No Helm releases in this cluster.
        </div>
      )}
      {data.releases.length > 0 && visible.length === 0 && (
        <div className="flex flex-1 items-center justify-center text-fg-muted">
          Nothing matches that filter.
        </div>
      )}
      {visible.length > 0 && (
        <div className={tableClass(selected !== null)}>
          <table className="w-full border-collapse text-left whitespace-nowrap">
            <thead className="sticky top-0 z-10 bg-surface-raised text-fg-muted">
              <tr className="border-b border-edge">
                <th className="px-2 py-1 font-medium">Name</th>
                <th className="px-2 py-1 font-medium">Namespace</th>
                <th className="px-2 py-1 font-medium">Chart</th>
                <th className="px-2 py-1 font-medium">App version</th>
                <th className="px-2 py-1 font-medium">
                  <span title="The latest chart version your configured Helm repositories offer">
                    Latest
                  </span>
                </th>
                <th className="px-2 py-1 text-right font-medium">Rev</th>
                <th className="px-2 py-1 font-medium">Status</th>
                <th className="px-2 py-1 text-right font-medium">Updated</th>
              </tr>
            </thead>
            <tbody>
              {visible.map((release) => (
                <tr key={releaseKey(release)} className={rowClass(sameRelease(selected, release))}>
                  <td className="truncate px-2 py-1 text-fg-strong" title={release.description}>
                    <button
                      type="button"
                      onClick={() => {
                        setSelected(release);
                      }}
                      className="max-w-full truncate hover:underline"
                    >
                      {release.name}
                    </button>
                  </td>
                  <td className="truncate px-2 py-1 text-fg-muted">{release.namespace}</td>
                  <td className="truncate px-2 py-1 text-fg-soft">{chartLabel(release)}</td>
                  <td className="truncate px-2 py-1 text-fg-muted">{orDash(release.appVersion)}</td>
                  <td className={`truncate px-2 py-1 ${latestColor(release)}`}>
                    {latestLabel(release)}
                    <span className="sr-only"> {latestNote(release)}</span>
                  </td>
                  <td className="px-2 py-1 text-right text-fg-muted">{release.revision}</td>
                  <td className="truncate px-2 py-1">
                    <span
                      className={`inline-flex items-center gap-1.5 ${statusText(release.status)}`}
                    >
                      <span className={`h-2 w-2 rounded-full ${statusDot(release.status)}`} />
                      {statusLabel(release.status)}
                    </span>
                  </td>
                  <td className="px-2 py-1 text-right text-fg-muted" title={release.updated}>
                    {ago(release.updated, now)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {selected !== null && (
        <HelmReleaseDetail
          key={releaseKey(selected)}
          release={selected}
          onSelectResource={onSelectResource}
          onChanged={reload}
          onClose={() => {
            setSelected(null);
          }}
        />
      )}
    </div>
  );
}

function tableClass(split: boolean): string {
  if (split) {
    return 'min-h-0 shrink-0 basis-2/5 overflow-auto';
  }
  return 'min-h-0 flex-1 overflow-auto';
}

function chartLabel(release: HelmRelease): string {
  if (release.chart === '') {
    return '—';
  }
  if (release.chartVersion === '') {
    return release.chart;
  }
  return `${release.chart}-${release.chartVersion}`;
}

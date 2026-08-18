import { Fragment } from 'react';
import type { ArgoApp } from '../lib/types';
import { refOf, useArgo } from '../lib/argocd';
import { healthClass, orDash, syncClass } from '../lib/argoStatus';
import { created } from '../lib/fluxStatus';
import LoadWarning from './LoadWarning';
import StaleBanner from './StaleBanner';

interface ArgoListProps {
  onSelect: (ref: ReturnType<typeof refOf>) => void;
}

interface KindGroup {
  name: string;
  resources: ArgoApp[];
}

function rowKey(app: ArgoApp): string {
  return `${app.resource}/${app.namespace}/${app.name}`;
}

function Row({
  app,
  onSelect,
}: {
  app: ArgoApp;
  onSelect: (ref: ReturnType<typeof refOf>) => void;
}) {
  return (
    <tr className="border-t border-edge hover:bg-surface-raised">
      <td className="truncate px-2 py-1 text-fg-strong">
        <button
          type="button"
          title={app.message}
          onClick={() => {
            onSelect(refOf(app));
          }}
          className="max-w-full truncate hover:underline"
        >
          {app.name}
        </button>
      </td>
      <td className="truncate px-2 py-1 text-fg-muted">{orDash(app.namespace)}</td>
      <td className="truncate px-2 py-1 text-fg-muted">{orDash(app.project)}</td>
      <td className={`truncate px-2 py-1 ${syncClass(app.sync)}`}>{orDash(app.sync)}</td>
      <td className={`truncate px-2 py-1 ${healthClass(app.health)}`}>{orDash(app.health)}</td>
      <td className="truncate px-2 py-1 text-fg-muted">{orDash(app.destination)}</td>
      <td className="truncate px-2 py-1 text-fg-muted" title={app.revision}>
        {orDash(app.revision)}
      </td>
      <td className="truncate px-2 py-1 text-fg-muted">{created(app.createdAt)}</td>
    </tr>
  );
}

const HEADERS = [
  { id: 'name', label: 'Name', width: 'w-56' },
  { id: 'namespace', label: 'Namespace', width: 'w-32' },
  { id: 'project', label: 'Project', width: 'w-32' },
  { id: 'sync', label: 'Sync', width: 'w-24' },
  { id: 'health', label: 'Health', width: 'w-24' },
  { id: 'destination', label: 'Destination', width: 'w-40' },
  { id: 'revision', label: 'Revision', width: 'w-40' },
  { id: 'created', label: 'Created', width: 'w-24' },
];

export default function ArgoList({ onSelect }: ArgoListProps) {
  const { data, error, reload } = useArgo();

  if (data === null) {
    if (error !== null) {
      return (
        <div className="flex h-full items-center justify-center text-xs text-error">{error}</div>
      );
    }
    return (
      <div className="flex h-full items-center justify-center text-xs text-fg-muted">
        Loading Argo CD resources
      </div>
    );
  }

  const groups: KindGroup[] = [
    { name: 'Applications', resources: data.apps },
    { name: 'ApplicationSets', resources: data.applicationSets },
    { name: 'Projects', resources: data.projects },
  ].filter((group) => group.resources.length > 0);

  return (
    <div className="flex h-full min-h-0 flex-col text-xs">
      {error !== null && <StaleBanner what="Argo CD resources" message={error} onRetry={reload} />}
      {data.error !== undefined && <LoadWarning message={data.error} />}
      {groups.length === 0 && (
        <div className="flex flex-1 items-center justify-center text-fg-muted">
          No Argo CD resources on this cluster.
        </div>
      )}
      {groups.length > 0 && (
        <div className="min-h-0 flex-1 overflow-auto">
          <table className="w-full table-fixed border-collapse text-left whitespace-nowrap">
            <thead className="sticky top-0 z-10 bg-surface-raised text-fg-muted">
              <tr className="border-b border-edge">
                {HEADERS.map((header) => (
                  <th key={header.id} className={`px-2 py-1 font-medium ${header.width}`}>
                    {header.label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {groups.map((group) => (
                <Fragment key={group.name}>
                  <tr>
                    <td
                      colSpan={HEADERS.length}
                      className="bg-surface-raised/50 px-2 pt-3 pb-1 font-semibold tracking-wide text-fg-soft uppercase"
                    >
                      {group.name}
                      <span className="ml-2 text-[11px] font-normal text-fg-muted">
                        {group.resources.length}
                      </span>
                    </td>
                  </tr>
                  {group.resources.map((app) => (
                    <Row key={rowKey(app)} app={app} onSelect={onSelect} />
                  ))}
                </Fragment>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

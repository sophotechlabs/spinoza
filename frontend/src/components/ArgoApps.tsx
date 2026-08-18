import { useArgo, refOf, tree } from '../lib/argocd';
import { healthClass, orDash, syncClass } from '../lib/argoStatus';
import type { ArgoApp } from '../lib/types';

interface ArgoAppsProps {
  onSelect: (ref: ReturnType<typeof refOf>) => void;
}

function Row({
  app,
  depth,
  onSelect,
}: {
  app: ArgoApp;
  depth: number;
  onSelect: (ref: ReturnType<typeof refOf>) => void;
}) {
  return (
    <button
      type="button"
      title={app.message}
      onClick={() => {
        onSelect(refOf(app));
      }}
      className="flex w-full items-baseline gap-3 border-b border-edge px-3 py-1.5 text-left hover:bg-surface-raised"
    >
      <span
        className="min-w-0 flex-1 truncate text-fg-strong"
        style={{ paddingLeft: `${String(depth * 16)}px` }}
      >
        {app.name}
      </span>
      <span className="w-24 shrink-0 truncate text-fg-muted">{orDash(app.namespace)}</span>
      <span className={`w-24 shrink-0 truncate ${syncClass(app.sync)}`}>{orDash(app.sync)}</span>
      <span className={`w-24 shrink-0 truncate ${healthClass(app.health)}`}>
        {orDash(app.health)}
      </span>
      <span className="w-40 shrink-0 truncate text-fg-muted">{orDash(app.destination)}</span>
      <span className="w-32 shrink-0 truncate text-fg-muted" title={app.revision}>
        {orDash(app.revision)}
      </span>
    </button>
  );
}

export default function ArgoApps({ onSelect }: ArgoAppsProps) {
  const { data, error } = useArgo();

  if (error !== null && data === null) {
    return (
      <p role="status" className="p-4 text-xs text-error">
        Argo CD could not be read: {error}
      </p>
    );
  }

  if (data === null) {
    return <div className="p-4 text-xs text-fg-muted">Loading Argo CD applications</div>;
  }

  const rows = tree(data.apps);

  return (
    <div className="flex h-full min-h-0 flex-col text-xs">
      {data.error !== undefined && (
        <p role="status" className="border-b border-edge px-3 py-1 text-warn">
          {data.error}
        </p>
      )}
      <div className="flex shrink-0 items-baseline gap-3 border-b border-edge px-3 py-1.5 text-fg-muted">
        <span className="min-w-0 flex-1">Application</span>
        <span className="w-24 shrink-0">Namespace</span>
        <span className="w-24 shrink-0">Sync</span>
        <span className="w-24 shrink-0">Health</span>
        <span className="w-40 shrink-0">Destination</span>
        <span className="w-32 shrink-0">Revision</span>
      </div>
      {rows.length === 0 && <p className="p-3 text-fg-muted">No applications on this cluster.</p>}
      <div className="min-h-0 flex-1 overflow-y-auto">
        {rows.map((row) => (
          <Row
            key={`${row.app.namespace}/${row.app.name}`}
            app={row.app}
            depth={row.depth}
            onSelect={onSelect}
          />
        ))}
      </div>
    </div>
  );
}

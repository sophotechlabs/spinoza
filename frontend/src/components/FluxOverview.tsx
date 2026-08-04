import { useFlux } from '../lib/flux';
import { groupSummary } from '../lib/readiness';
import type { FluxGroup, FluxResource } from '../lib/types';
import LoadWarning from './LoadWarning';
import LoadFailure from './LoadFailure';
import { created, statusDot, statusLabel, statusText } from '../lib/fluxStatus';

function Tile({
  resource,
  onSelect,
}: {
  resource: FluxResource;
  onSelect: (resource: FluxResource) => void;
}) {
  function handleSelect() {
    onSelect(resource);
  }

  return (
    <button
      type="button"
      onClick={handleSelect}
      className="rounded border border-edge bg-surface-raised p-2.5 text-left hover:border-edge-strong"
      title={resource.message}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="truncate text-sm text-fg-strong">{resource.name}</span>
        <span className={`h-2 w-2 shrink-0 rounded-full ${statusDot(resource)}`} />
      </div>
      <div className="mt-0.5 truncate text-[11px] text-fg-muted">
        {resource.kind} · {resource.namespace}
      </div>
      <div className={`mt-2 text-[11px] ${statusText(resource)}`}>{statusLabel(resource)}</div>
      <div className="mt-1 truncate text-[11px] text-fg-muted" title={resource.revision}>
        {resource.revision}
      </div>
      <div className="truncate text-[11px] text-fg-muted">{resource.source}</div>
      <div className="mt-1 text-[10px] text-fg-muted">{created(resource.createdAt)}</div>
    </button>
  );
}

function TileGroup({
  group,
  onSelect,
}: {
  group: FluxGroup;
  onSelect: (resource: FluxResource) => void;
}) {
  return (
    <section className="mb-5">
      <h2 className="mb-2 flex items-center gap-2 px-1 text-xs font-semibold tracking-wide text-fg-soft uppercase">
        {group.name}
        <span className="text-[11px] font-normal text-fg-muted">{groupSummary(group)}</span>
      </h2>
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        {group.resources.map((resource) => (
          <Tile
            key={`${resource.kind}/${resource.namespace}/${resource.name}`}
            resource={resource}
            onSelect={onSelect}
          />
        ))}
      </div>
    </section>
  );
}

interface FluxOverviewProps {
  onSelect: (resource: FluxResource) => void;
}

export default function FluxOverview({ onSelect }: FluxOverviewProps) {
  const { data, error } = useFlux();

  if (data === null) {
    if (error !== null) {
      return (
        <div className="flex h-full items-center justify-center text-xs text-error">{error}</div>
      );
    }
    return (
      <div className="flex h-full items-center justify-center text-xs text-fg-muted">
        Loading Flux resources…
      </div>
    );
  }

  if (data.groups.length === 0) {
    if (data.error !== undefined) {
      return <LoadFailure what="Flux resources" message={data.error} />;
    }
    return (
      <div className="flex h-full items-center justify-center text-xs text-fg-muted">
        No Flux resources found.
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      {data.error !== undefined && <LoadWarning message={data.error} />}
      <div className="min-h-0 flex-1 overflow-auto p-3">
        {data.groups.map((group) => (
          <TileGroup key={group.name} group={group} onSelect={onSelect} />
        ))}
      </div>
    </div>
  );
}

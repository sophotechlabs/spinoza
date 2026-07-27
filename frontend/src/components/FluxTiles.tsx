import { useFlux } from '../lib/flux';
import type { FluxGroup, FluxResource } from '../lib/types';
import { created, statusDot, statusLabel, statusText } from '../lib/fluxStatus';

function Tile({ resource }: { resource: FluxResource }) {
  return (
    <div
      className="rounded border border-neutral-800 bg-neutral-900 p-2.5 hover:border-neutral-700"
      title={resource.message}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="truncate text-sm text-neutral-100">{resource.name}</span>
        <span className={`h-2 w-2 shrink-0 rounded-full ${statusDot(resource)}`} />
      </div>
      <div className="mt-0.5 truncate text-[11px] text-neutral-500">
        {resource.kind} · {resource.namespace}
      </div>
      <div className={`mt-2 text-[11px] ${statusText(resource)}`}>{statusLabel(resource)}</div>
      <div className="mt-1 truncate text-[11px] text-neutral-400" title={resource.revision}>
        {resource.revision}
      </div>
      <div className="truncate text-[11px] text-neutral-500">{resource.source}</div>
      <div className="mt-1 text-[10px] text-neutral-600">{created(resource.createdAt)}</div>
    </div>
  );
}

function TileGroup({ group }: { group: FluxGroup }) {
  return (
    <section className="mb-5">
      <h2 className="mb-2 flex items-center gap-2 px-1 text-xs font-semibold tracking-wide text-neutral-300 uppercase">
        {group.name}
        <span className="text-[11px] font-normal text-neutral-600">
          {group.ready}/{group.total} ready
        </span>
      </h2>
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        {group.resources.map((resource) => (
          <Tile
            key={`${resource.kind}/${resource.namespace}/${resource.name}`}
            resource={resource}
          />
        ))}
      </div>
    </section>
  );
}

export default function FluxTiles() {
  const { data, error } = useFlux();

  if (data === null) {
    if (error !== null) {
      return (
        <div className="flex h-full items-center justify-center text-xs text-red-400">{error}</div>
      );
    }
    return (
      <div className="flex h-full items-center justify-center text-xs text-neutral-600">
        Loading Flux resources…
      </div>
    );
  }

  if (data.groups.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-xs text-neutral-600">
        No Flux resources found.
      </div>
    );
  }

  return (
    <div className="h-full overflow-auto p-3">
      {data.groups.map((group) => (
        <TileGroup key={group.name} group={group} />
      ))}
    </div>
  );
}

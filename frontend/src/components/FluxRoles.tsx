import { useState } from 'react';
import { useFlux } from '../lib/flux';
import { allReady, readyOf, readySummary, reportingOf } from '../lib/readiness';
import type { FluxResource } from '../lib/types';
import { created, statusDot, statusLabel, statusText } from '../lib/fluxStatus';

interface Section {
  name: string;
  kinds: string[];
}

const SECTIONS: Section[] = [
  { name: 'Appliers', kinds: ['Kustomization', 'HelmRelease'] },
  {
    name: 'Sources',
    kinds: ['GitRepository', 'OCIRepository', 'HelmRepository', 'HelmChart', 'Bucket'],
  },
  { name: 'Image Automation', kinds: ['ImageRepository', 'ImagePolicy', 'ImageUpdateAutomation'] },
  { name: 'Notifications', kinds: ['Provider', 'Alert', 'Receiver'] },
];

const KIND_GROUP: Record<string, string> = {
  Kustomization: 'kustomize.toolkit.fluxcd.io',
  HelmRelease: 'helm.toolkit.fluxcd.io',
  GitRepository: 'source.toolkit.fluxcd.io',
  OCIRepository: 'source.toolkit.fluxcd.io',
  HelmRepository: 'source.toolkit.fluxcd.io',
  HelmChart: 'source.toolkit.fluxcd.io',
  Bucket: 'source.toolkit.fluxcd.io',
  ImageRepository: 'image.toolkit.fluxcd.io',
  ImagePolicy: 'image.toolkit.fluxcd.io',
  ImageUpdateAutomation: 'image.toolkit.fluxcd.io',
  Provider: 'notification.toolkit.fluxcd.io',
  Alert: 'notification.toolkit.fluxcd.io',
  Receiver: 'notification.toolkit.fluxcd.io',
};

function groupOf(kind: string): string {
  return KIND_GROUP[kind];
}

function byKind(resources: FluxResource[]): Map<string, FluxResource[]> {
  const map = new Map<string, FluxResource[]>();
  for (const resource of resources) {
    const existing = map.get(resource.kind);
    if (existing === undefined) {
      map.set(resource.kind, [resource]);
    } else {
      existing.push(resource);
    }
  }
  return map;
}

function kindResources(map: Map<string, FluxResource[]>, kind: string): FluxResource[] {
  const list = map.get(kind);
  if (list === undefined) {
    return [];
  }
  return list;
}

function tileBorder(ready: number, reporting: number, count: number): string {
  if (count === 0) {
    return 'border-neutral-800';
  }
  if (allReady(ready, reporting, count)) {
    return 'border-green-800';
  }
  if (reporting === 0) {
    return 'border-neutral-800';
  }
  return 'border-amber-800';
}

function KindTile({
  kind,
  resources,
  onSelect,
}: {
  kind: string;
  resources: FluxResource[];
  onSelect: () => void;
}) {
  const count = resources.length;
  const ready = readyOf(resources);
  const reporting = reportingOf(resources);
  return (
    <button
      type="button"
      onClick={onSelect}
      className={`rounded border ${tileBorder(ready, reporting, count)} bg-neutral-900 p-2.5 text-left hover:bg-neutral-800`}
    >
      <div className="truncate text-sm font-semibold text-neutral-100">{kind}</div>
      <div className="truncate text-[10px] text-neutral-500">{groupOf(kind)}</div>
      <div className="mt-2 text-xl font-semibold text-neutral-100">{count}</div>
      <div className="mt-0.5 text-[10px] text-neutral-500">
        {readySummary(ready, reporting, count)}
      </div>
    </button>
  );
}

function KindList({
  kind,
  resources,
  onBack,
  onSelect,
}: {
  kind: string;
  resources: FluxResource[];
  onBack: () => void;
  onSelect: (resource: FluxResource) => void;
}) {
  return (
    <div className="h-full overflow-auto p-3">
      <button
        type="button"
        onClick={onBack}
        className="mb-2 text-xs text-neutral-400 hover:text-neutral-200"
      >
        ← Flux Resources
      </button>
      <div className="mb-2 text-xs text-neutral-500">
        {kind} · {resources.length} resources
      </div>
      <div className="border-t border-neutral-900">
        {resources.map((resource) => (
          <button
            type="button"
            key={`${resource.namespace}/${resource.name}`}
            onClick={() => {
              onSelect(resource);
            }}
            className="flex w-full items-center gap-2 border-b border-neutral-900 px-2 py-1.5 text-left hover:bg-neutral-900"
            title={resource.message}
          >
            <span className={`h-2 w-2 shrink-0 rounded-full ${statusDot(resource)}`} />
            <span className="text-neutral-500">{resource.namespace}/</span>
            <span className="truncate text-neutral-100">{resource.name}</span>
            <span className={`ml-2 shrink-0 text-[11px] ${statusText(resource)}`}>
              {statusLabel(resource)}
            </span>
            <span className="ml-auto shrink-0 truncate pl-4 text-[11px] text-neutral-500">
              {resource.revision}
            </span>
            <span className="shrink-0 text-[11px] text-neutral-600">
              {created(resource.createdAt)}
            </span>
          </button>
        ))}
      </div>
    </div>
  );
}

interface FluxRolesProps {
  onSelect: (resource: FluxResource) => void;
}

export default function FluxRoles({ onSelect }: FluxRolesProps) {
  const { data, error } = useFlux();
  const [kind, setKind] = useState<string | null>(null);

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

  const map = byKind(data.groups.flatMap((group) => group.resources));

  if (kind !== null) {
    return (
      <KindList
        kind={kind}
        resources={kindResources(map, kind)}
        onBack={() => {
          setKind(null);
        }}
        onSelect={onSelect}
      />
    );
  }

  return (
    <div className="h-full overflow-auto p-3">
      {SECTIONS.map((section) => (
        <section key={section.name} className="mb-5">
          <h2 className="mb-2 px-1 text-xs font-semibold tracking-wide text-neutral-300 uppercase">
            {section.name}
          </h2>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
            {section.kinds.map((sectionKind) => (
              <KindTile
                key={sectionKind}
                kind={sectionKind}
                resources={kindResources(map, sectionKind)}
                onSelect={() => {
                  setKind(sectionKind);
                }}
              />
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}

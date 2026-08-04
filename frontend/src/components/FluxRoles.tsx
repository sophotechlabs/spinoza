import { useState } from 'react';
import type { ReactNode } from 'react';
import { useFlux } from '../lib/flux';
import { allReady, readyOf, readySummary, reportingOf } from '../lib/readiness';
import type { FluxResource } from '../lib/types';
import { created, statusDot, statusLabel, statusText } from '../lib/fluxStatus';
import StaleBanner from './StaleBanner';

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
    return 'border-edge';
  }
  if (allReady(ready, reporting, count)) {
    return 'border-ok-line-strong';
  }
  if (reporting === 0) {
    return 'border-edge';
  }
  return 'border-warn-line-strong';
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
      className={`rounded border ${tileBorder(ready, reporting, count)} bg-surface-raised p-2.5 text-left hover:bg-surface-active`}
    >
      <div className="truncate text-sm font-semibold text-fg-strong">{kind}</div>
      <div className="truncate text-[10px] text-fg-muted">{groupOf(kind)}</div>
      <div className="mt-2 text-xl font-semibold text-fg-strong">{count}</div>
      <div className="mt-0.5 text-[10px] text-fg-muted">
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
    <div className="min-h-0 flex-1 overflow-auto p-3">
      <button type="button" onClick={onBack} className="mb-2 text-xs text-fg-muted hover:text-fg">
        ← Flux Resources
      </button>
      <div className="mb-2 text-xs text-fg-muted">
        {kind} · {resources.length} resources
      </div>
      <div className="border-t border-edge">
        {resources.map((resource) => (
          <button
            type="button"
            key={`${resource.namespace}/${resource.name}`}
            onClick={() => {
              onSelect(resource);
            }}
            className="flex w-full items-center gap-2 border-b border-edge px-2 py-1.5 text-left hover:bg-surface-raised"
            title={resource.message}
          >
            <span className={`h-2 w-2 shrink-0 rounded-full ${statusDot(resource)}`} />
            <span className="text-fg-muted">{resource.namespace}/</span>
            <span className="truncate text-fg-strong">{resource.name}</span>
            <span className={`ml-2 shrink-0 text-[11px] ${statusText(resource)}`}>
              {statusLabel(resource)}
            </span>
            <span className="ml-auto shrink-0 truncate pl-4 text-[11px] text-fg-muted">
              {resource.revision}
            </span>
            <span className="shrink-0 text-[11px] text-fg-muted">
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
  const { data, error, reload } = useFlux();
  const [kind, setKind] = useState<string | null>(null);

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

  let notice: ReactNode = null;
  if (error !== null) {
    notice = <StaleBanner what="Flux resources" message={error} onRetry={reload} />;
  }

  const map = byKind(data.groups.flatMap((group) => group.resources));

  if (kind !== null) {
    return (
      <div className="flex h-full min-h-0 flex-col">
        {notice}
        <KindList
          kind={kind}
          resources={kindResources(map, kind)}
          onBack={() => {
            setKind(null);
          }}
          onSelect={onSelect}
        />
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      {notice}
      <div className="min-h-0 flex-1 overflow-auto p-3">
        {SECTIONS.map((section) => (
          <section key={section.name} className="mb-5">
            <h2 className="mb-2 px-1 text-xs font-semibold tracking-wide text-fg-soft uppercase">
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
    </div>
  );
}

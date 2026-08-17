import { useEffect, useState } from 'react';
import type { Category, ResourceDescriptor, View } from '../lib/types';
import { fetchResourceCounts, fetchResources, refreshResources } from '../lib/discovery';
import { groupByApiGroup, isNested } from '../lib/sidebarTree';
import { NUDGE_STEP, useSidebarWidth } from '../lib/usePanelWidth';
import { useClusterEpoch } from '../store/cluster';
import {
  ARGO_SECTION,
  FLUX_SECTION,
  readSections,
  sectionOpen,
  writeSections,
} from '../lib/sidebarState';
import { argoInstalled, argoTypes, fluxInstalled } from '../lib/gitops';
import type { SidebarSections } from '../lib/sidebarState';

interface SidebarProps {
  view: View;
  activeResource: ResourceDescriptor | null;
  onSelect: (descriptor: ResourceDescriptor) => void;
  onSelectView: (view: View) => void;
}

interface GitopsEntry {
  view: View;
  label: string;
}

const TOP_VIEWS: GitopsEntry[] = [
  { view: 'cluster', label: 'Cluster' },
  { view: 'helm', label: 'Helm releases' },
];

const FLUX_VIEWS: GitopsEntry[] = [
  { view: 'flux-roles', label: 'Overview' },
  { view: 'gitops', label: 'Graph' },
  { view: 'flux-list', label: 'Resource list' },
];

const NOT_INSTALLED = 'not found in this cluster';

function descriptorKey(descriptor: ResourceDescriptor): string {
  return `${descriptor.group}/${descriptor.version}/${descriptor.resource}`;
}

function isActive(active: ResourceDescriptor | null, descriptor: ResourceDescriptor): boolean {
  if (active === null) {
    return false;
  }
  return descriptorKey(active) === descriptorKey(descriptor);
}

function chevron(collapsed: boolean): string {
  if (collapsed) {
    return '▸';
  }
  return '▾';
}

function current(active: boolean): 'page' | undefined {
  if (active) {
    return 'page';
  }
  return undefined;
}

function resourceClass(active: boolean, nested = false, empty = false): string {
  let indent = 'pl-6';
  if (nested) {
    indent = 'pl-9';
  }
  const base = `flex w-full items-center justify-between gap-1 ${indent} pr-3 py-1 text-left`;
  if (active) {
    return `${base} bg-surface-active text-fg-strong`;
  }
  if (empty) {
    return `${base} text-fg-subtle hover:bg-surface-raised`;
  }
  return `${base} text-fg-soft hover:bg-surface-raised`;
}

function countLabel(count: number | undefined): string {
  if (count === undefined) {
    return '';
  }
  if (count < 0) {
    return '-';
  }
  return String(count);
}

function isEmpty(count: number | undefined): boolean {
  return count === 0;
}

function byPopulated(
  resources: ResourceDescriptor[],
  counts: Record<string, number>,
): ResourceDescriptor[] {
  const populated = resources.filter((one) => !isEmpty(counts[descriptorKey(one)]));
  const empty = resources.filter((one) => isEmpty(counts[descriptorKey(one)]));
  return [...populated, ...empty];
}

function kindTitle(kind: string, total: number | undefined, failing: number | undefined): string {
  if (failing === undefined) {
    return kind;
  }
  const totalLabel = countLabel(total);
  if (totalLabel === '' || totalLabel === '-') {
    return `${kind}: ${String(failing)} not ready`;
  }
  return `${kind}: ${String(failing)} of ${totalLabel} not ready`;
}

function failingNote(failing: number | undefined): string {
  if (failing === undefined) {
    return '';
  }
  return `, ${String(failing)} not ready`;
}

function failingBadge(failing: number | undefined): string {
  if (failing === undefined) {
    return '';
  }
  return `(${String(failing)})`;
}

function errorMessage(err: unknown, fallback: string): string {
  if (err instanceof Error) {
    return err.message;
  }
  return fallback;
}

function retryLabel(retrying: boolean): string {
  if (retrying) {
    return 'Retrying';
  }
  return 'Retry';
}

const sectionClass =
  'flex w-full items-center justify-between px-3 py-1 text-[11px] font-semibold tracking-wide text-fg-muted uppercase hover:text-fg';

const missingClass =
  'flex w-full items-center justify-between px-3 py-1 text-[11px] font-semibold tracking-wide text-fg-faint uppercase';

function engineClass(found: boolean): string {
  if (found) {
    return sectionClass;
  }
  return missingClass;
}

function engineMark(found: boolean, open: boolean): string {
  if (!found) {
    return '';
  }
  return chevron(!open);
}

export default function Sidebar({ view, activeResource, onSelect, onSelectView }: SidebarProps) {
  const epoch = useClusterEpoch();
  const { size: width, startResize, nudge } = useSidebarWidth();
  const [categories, setCategories] = useState<Category[]>([]);
  const flux = fluxInstalled(categories);
  const argo = argoInstalled(categories);
  const [sections, setSections] = useState<SidebarSections>(() => readSections());
  const [error, setError] = useState<string | null>(null);
  const [countsError, setCountsError] = useState<string | null>(null);
  const [retrying, setRetrying] = useState(false);
  const [counts, setCounts] = useState<Record<string, number>>({});
  const [failing, setFailing] = useState<Record<string, number>>({});

  useEffect(() => {
    let mounted = true;
    setCounts({});
    setFailing({});
    setCountsError(null);
    const load = async () => {
      try {
        const catalog = await fetchResources();
        if (!mounted) {
          return;
        }
        setCategories(catalog.categories);
        setError(catalog.error ?? null);
      } catch (err: unknown) {
        if (mounted) {
          setError(errorMessage(err, 'discovery request failed'));
        }
        return;
      }
      await loadCounts(() => mounted);
    };
    void load();
    return () => {
      mounted = false;
    };
  }, [epoch]);

  async function loadCounts(live: () => boolean) {
    try {
      const tally = await fetchResourceCounts();
      if (live()) {
        setCounts(tally.counts);
        setFailing(tally.failing ?? {});
        setCountsError(null);
      }
    } catch (err: unknown) {
      if (live()) {
        setCountsError(errorMessage(err, 'resource counts request failed'));
      }
    }
  }

  async function retry() {
    setRetrying(true);
    try {
      const catalog = await refreshResources();
      setCategories(catalog.categories);
      setError(catalog.error ?? null);
    } catch (err: unknown) {
      setError(errorMessage(err, 'discovery request failed'));
      setRetrying(false);
      return;
    }
    await loadCounts(() => true);
    setRetrying(false);
  }

  function toggle(name: string) {
    const next = { ...sections, [name]: !sectionOpen(sections, name) };
    setSections(next);
    writeSections(next);
  }

  function handleResize(event: React.MouseEvent<HTMLButtonElement>) {
    event.preventDefault();
    startResize(event.clientX);
  }

  function handleResizeKey(event: React.KeyboardEvent<HTMLButtonElement>) {
    if (event.key === 'ArrowRight') {
      event.preventDefault();
      nudge(NUDGE_STEP);
      return;
    }
    if (event.key === 'ArrowLeft') {
      event.preventDefault();
      nudge(-NUDGE_STEP);
    }
  }

  return (
    <div
      style={{ width: `${width}px` }}
      className="flex min-h-0 shrink-0 border-r border-edge bg-surface"
    >
      <nav className="min-w-0 flex-1 overflow-y-auto py-2">
        <div className="mb-1" aria-label="Cluster views">
          {TOP_VIEWS.map((entry) => (
            <button
              key={entry.view}
              type="button"
              aria-current={current(view === entry.view)}
              onClick={() => {
                onSelectView(entry.view);
              }}
              className={resourceClass(view === entry.view)}
            >
              {entry.label}
            </button>
          ))}
        </div>
        <div className="mb-1">
          <button
            type="button"
            disabled={!flux}
            title={flux ? undefined : `Flux is ${NOT_INSTALLED}`}
            aria-expanded={flux && sectionOpen(sections, FLUX_SECTION)}
            onClick={() => {
              toggle(FLUX_SECTION);
            }}
            className={engineClass(flux)}
          >
            <span>
              <span aria-hidden="true">
                {engineMark(flux, sectionOpen(sections, FLUX_SECTION))}
              </span>{' '}
              {FLUX_SECTION}
            </span>
          </button>
          {flux && sectionOpen(sections, FLUX_SECTION) && (
            <div aria-label="Flux views">
              {FLUX_VIEWS.map((entry) => (
                <button
                  key={entry.view}
                  type="button"
                  aria-label={`Flux ${entry.label}`}
                  aria-current={current(view === entry.view)}
                  onClick={() => {
                    onSelectView(entry.view);
                  }}
                  className={resourceClass(view === entry.view)}
                >
                  {entry.label}
                </button>
              ))}
            </div>
          )}
        </div>
        <div className="mb-1">
          <button
            type="button"
            disabled={!argo}
            title={argo ? undefined : `Argo CD is ${NOT_INSTALLED}`}
            aria-expanded={argo && sectionOpen(sections, ARGO_SECTION)}
            onClick={() => {
              toggle(ARGO_SECTION);
            }}
            className={engineClass(argo)}
          >
            <span>
              <span aria-hidden="true">
                {engineMark(argo, sectionOpen(sections, ARGO_SECTION))}
              </span>{' '}
              {ARGO_SECTION}
            </span>
          </button>
          {argo && sectionOpen(sections, ARGO_SECTION) && (
            <div aria-label="Argo CD views">
              <button
                type="button"
                aria-label="Argo CD Applications"
                aria-current={current(view === 'argo-apps')}
                onClick={() => {
                  onSelectView('argo-apps');
                }}
                className={resourceClass(view === 'argo-apps')}
              >
                Applications
              </button>
              {argoTypes(categories).map((descriptor) => (
                <button
                  key={descriptorKey(descriptor)}
                  type="button"
                  aria-current={current(isActive(activeResource, descriptor))}
                  onClick={() => {
                    onSelect(descriptor);
                  }}
                  className={resourceClass(isActive(activeResource, descriptor))}
                >
                  {descriptor.kind}
                </button>
              ))}
            </div>
          )}
        </div>
        {error !== null && (
          <div
            role="alert"
            className="mx-2 mb-1 rounded border border-error-line bg-error-tint/40 px-2 py-1.5 text-[11px]"
          >
            <div className="font-semibold text-error">Discovery failed</div>
            <div className="mt-0.5 break-words text-error-strong">{error}</div>
            <button
              type="button"
              onClick={() => void retry()}
              disabled={retrying}
              className="mt-1.5 rounded border border-error-line-strong px-1.5 py-0.5 text-error-contrast hover:bg-error-tint-strong disabled:cursor-not-allowed disabled:text-error-muted"
            >
              {retryLabel(retrying)}
            </button>
          </div>
        )}
        {countsError !== null && (
          <div
            role="status"
            title={countsError}
            className="mx-2 mb-1 truncate text-[11px] text-warn-muted"
          >
            Object counts unavailable: {countsError}
          </div>
        )}
        {error === null && categories.length === 0 && (
          <div className="px-3 py-1 text-[11px] text-fg-muted">No resource types discovered.</div>
        )}
        {categories.map((category) => {
          const isCollapsed = !sectionOpen(sections, category.name);
          return (
            <div key={category.name} className="mb-1">
              <button
                type="button"
                aria-expanded={!isCollapsed}
                onClick={() => {
                  toggle(category.name);
                }}
                className={sectionClass}
              >
                <span>
                  <span aria-hidden="true">{chevron(isCollapsed)}</span> {category.name}
                </span>
                <span className="text-fg-muted">{category.resources.length}</span>
              </button>
              {!isCollapsed && !isNested(category.name) && (
                <div>
                  {byPopulated(category.resources, counts).map((resource) => (
                    <button
                      key={descriptorKey(resource)}
                      type="button"
                      aria-current={current(isActive(activeResource, resource))}
                      onClick={() => {
                        onSelect(resource);
                      }}
                      title={kindTitle(
                        resource.kind,
                        counts[descriptorKey(resource)],
                        failing[descriptorKey(resource)],
                      )}
                      className={resourceClass(
                        isActive(activeResource, resource),
                        false,
                        isEmpty(counts[descriptorKey(resource)]),
                      )}
                    >
                      <span className="truncate">{resource.kind}</span>{' '}
                      <span className="flex shrink-0 items-center gap-1 text-fg-subtle">
                        {failingBadge(failing[descriptorKey(resource)]) !== '' && (
                          <span aria-hidden="true" className="text-error">
                            {failingBadge(failing[descriptorKey(resource)])}
                          </span>
                        )}
                        <span>{countLabel(counts[descriptorKey(resource)])}</span>
                        <span className="sr-only">
                          {failingNote(failing[descriptorKey(resource)])}
                        </span>
                      </span>
                    </button>
                  ))}
                </div>
              )}
              {!isCollapsed && isNested(category.name) && (
                <div>
                  {groupByApiGroup(category.resources).map((group) => {
                    const key = `${category.name}/${group.name}`;
                    const groupCollapsed = !sectionOpen(sections, key);
                    return (
                      <div key={key}>
                        <button
                          type="button"
                          aria-expanded={!groupCollapsed}
                          onClick={() => {
                            toggle(key);
                          }}
                          title={group.name}
                          className="flex w-full items-center justify-between gap-1 py-1 pr-3 pl-5 text-left text-fg-muted hover:bg-surface-raised hover:text-fg"
                        >
                          <span className="truncate">
                            <span aria-hidden="true">{chevron(groupCollapsed)}</span> {group.name}
                          </span>
                          <span className="shrink-0 text-fg-muted">{group.resources.length}</span>
                        </button>
                        {!groupCollapsed && (
                          <div>
                            {byPopulated(group.resources, counts).map((resource) => (
                              <button
                                key={descriptorKey(resource)}
                                type="button"
                                aria-current={current(isActive(activeResource, resource))}
                                onClick={() => {
                                  onSelect(resource);
                                }}
                                title={kindTitle(
                                  resource.kind,
                                  counts[descriptorKey(resource)],
                                  failing[descriptorKey(resource)],
                                )}
                                className={resourceClass(
                                  isActive(activeResource, resource),
                                  true,
                                  isEmpty(counts[descriptorKey(resource)]),
                                )}
                              >
                                <span className="truncate">{resource.kind}</span>{' '}
                                <span className="flex shrink-0 items-center gap-1 text-fg-subtle">
                                  {failingBadge(failing[descriptorKey(resource)]) !== '' && (
                                    <span aria-hidden="true" className="text-error">
                                      {failingBadge(failing[descriptorKey(resource)])}
                                    </span>
                                  )}
                                  <span>{countLabel(counts[descriptorKey(resource)])}</span>
                                  <span className="sr-only">
                                    {failingNote(failing[descriptorKey(resource)])}
                                  </span>
                                </span>
                              </button>
                            ))}
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          );
        })}
      </nav>
      <button
        type="button"
        aria-label="Resize sidebar"
        onMouseDown={handleResize}
        onKeyDown={handleResizeKey}
        className="w-1 shrink-0 cursor-col-resize bg-handle hover:bg-handle-active"
      />
    </div>
  );
}

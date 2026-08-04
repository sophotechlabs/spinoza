import { useEffect, useState } from 'react';
import type { Category, ResourceDescriptor, View } from '../lib/types';
import { fetchResourceCounts, fetchResources, refreshResources } from '../lib/discovery';
import { groupByApiGroup, isNested } from '../lib/sidebarTree';
import { NUDGE_STEP, useSidebarWidth } from '../lib/usePanelWidth';
import { useClusterEpoch } from '../store/cluster';

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

const GITOPS_VIEWS: GitopsEntry[] = [
  { view: 'flux-roles', label: 'Overview' },
  { view: 'gitops', label: 'Graph' },
  { view: 'flux-list', label: 'Resource list' },
  { view: 'flux-overview', label: 'Status tiles' },
];

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
  let indent = 'px-6';
  if (nested) {
    indent = 'px-9';
  }
  const base = `flex w-full items-center justify-between gap-1 ${indent} py-1 text-left`;
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
    return '—';
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

function collapsedKeys(categories: Category[]): Set<string> {
  const keys = new Set<string>();
  for (const category of categories) {
    keys.add(category.name);
    if (!isNested(category.name)) {
      continue;
    }
    for (const group of groupByApiGroup(category.resources)) {
      keys.add(`${category.name}/${group.name}`);
    }
  }
  return keys;
}

function errorMessage(err: unknown, fallback: string): string {
  if (err instanceof Error) {
    return err.message;
  }
  return fallback;
}

function retryLabel(retrying: boolean): string {
  if (retrying) {
    return 'Retrying…';
  }
  return 'Retry';
}

const sectionClass =
  'flex w-full items-center justify-between px-3 py-1 text-[11px] font-semibold tracking-wide text-fg-muted uppercase hover:text-fg';

export default function Sidebar({ view, activeResource, onSelect, onSelectView }: SidebarProps) {
  const epoch = useClusterEpoch();
  const { size: width, startResize, nudge } = useSidebarWidth();
  const [categories, setCategories] = useState<Category[]>([]);
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const [error, setError] = useState<string | null>(null);
  const [countsError, setCountsError] = useState<string | null>(null);
  const [retrying, setRetrying] = useState(false);
  const [counts, setCounts] = useState<Record<string, number>>({});

  useEffect(() => {
    let mounted = true;
    setCounts({});
    setCountsError(null);
    const load = async () => {
      try {
        const catalog = await fetchResources();
        if (!mounted) {
          return;
        }
        setCategories(catalog.categories);
        setCollapsed(collapsedKeys(catalog.categories));
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
        setCounts(tally);
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
      setCollapsed(collapsedKeys(catalog.categories));
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
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(name)) {
        next.delete(name);
      } else {
        next.add(name);
      }
      return next;
    });
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

  const gitopsCollapsed = collapsed.has('GitOps');

  return (
    <div
      style={{ width: `${width}px` }}
      className="flex min-h-0 shrink-0 border-r border-edge bg-surface"
    >
      <nav className="min-w-0 flex-1 overflow-y-auto py-2">
        <div className="mb-1">
          <button
            type="button"
            aria-expanded={!gitopsCollapsed}
            onClick={() => {
              toggle('GitOps');
            }}
            className={sectionClass}
          >
            <span>
              <span aria-hidden="true">{chevron(gitopsCollapsed)}</span> GitOps
            </span>
          </button>
          {!gitopsCollapsed && (
            <div aria-label="GitOps views">
              {GITOPS_VIEWS.map((entry) => (
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
            Object counts unavailable — {countsError}
          </div>
        )}
        {error === null && categories.length === 0 && (
          <div className="px-3 py-1 text-[11px] text-fg-muted">No resource types discovered.</div>
        )}
        {categories.map((category) => {
          const isCollapsed = collapsed.has(category.name);
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
                      title={resource.kind}
                      className={resourceClass(
                        isActive(activeResource, resource),
                        false,
                        isEmpty(counts[descriptorKey(resource)]),
                      )}
                    >
                      <span className="truncate">{resource.kind}</span>{' '}
                      <span className="shrink-0 text-fg-subtle">
                        {countLabel(counts[descriptorKey(resource)])}
                      </span>
                    </button>
                  ))}
                </div>
              )}
              {!isCollapsed && isNested(category.name) && (
                <div>
                  {groupByApiGroup(category.resources).map((group) => {
                    const key = `${category.name}/${group.name}`;
                    const groupCollapsed = collapsed.has(key);
                    return (
                      <div key={key}>
                        <button
                          type="button"
                          aria-expanded={!groupCollapsed}
                          onClick={() => {
                            toggle(key);
                          }}
                          title={group.name}
                          className="flex w-full items-center justify-between gap-1 px-5 py-1 text-left text-fg-muted hover:bg-surface-raised hover:text-fg"
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
                                title={resource.kind}
                                className={resourceClass(
                                  isActive(activeResource, resource),
                                  true,
                                  isEmpty(counts[descriptorKey(resource)]),
                                )}
                              >
                                <span className="truncate">{resource.kind}</span>{' '}
                                <span className="shrink-0 text-fg-subtle">
                                  {countLabel(counts[descriptorKey(resource)])}
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

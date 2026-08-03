import { useEffect, useState } from 'react';
import type { Category, ResourceDescriptor, View } from '../lib/types';
import { fetchResources, refreshResources } from '../lib/discovery';
import { groupByApiGroup, isNested } from '../lib/sidebarTree';
import { NUDGE_STEP, useSidebarWidth } from '../lib/usePanelWidth';

interface SidebarProps {
  epoch?: number;
  view: View;
  activeResource: ResourceDescriptor | null;
  onSelect: (descriptor: ResourceDescriptor) => void;
  onSelectGraph: () => void;
  onSelectList: () => void;
  onSelectOverview: () => void;
  onSelectRoles: () => void;
}

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

function resourceClass(active: boolean, nested = false): string {
  let indent = 'px-6';
  if (nested) {
    indent = 'px-9';
  }
  const base = `block w-full truncate ${indent} py-1 text-left`;
  if (active) {
    return `${base} bg-neutral-800 text-neutral-100`;
  }
  return `${base} text-neutral-300 hover:bg-neutral-900`;
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

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'discovery request failed';
}

function retryLabel(retrying: boolean): string {
  if (retrying) {
    return 'Retrying…';
  }
  return 'Retry';
}

const sectionClass =
  'flex w-full items-center justify-between px-3 py-1 text-[11px] font-semibold tracking-wide text-neutral-400 uppercase hover:text-neutral-200';

export default function Sidebar({
  epoch,
  view,
  activeResource,
  onSelect,
  onSelectGraph,
  onSelectList,
  onSelectOverview,
  onSelectRoles,
}: SidebarProps) {
  const { size: width, startResize, nudge } = useSidebarWidth();
  const [categories, setCategories] = useState<Category[]>([]);
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const [error, setError] = useState<string | null>(null);
  const [retrying, setRetrying] = useState(false);

  useEffect(() => {
    let mounted = true;
    const load = async () => {
      try {
        const catalog = await fetchResources();
        if (mounted) {
          setCategories(catalog.categories);
          setCollapsed(collapsedKeys(catalog.categories));
          setError(catalog.error ?? null);
        }
      } catch (err: unknown) {
        if (mounted) {
          setError(errorMessage(err));
        }
      }
    };
    void load();
    return () => {
      mounted = false;
    };
  }, [epoch]);

  async function retry() {
    setRetrying(true);
    try {
      const catalog = await refreshResources();
      setCategories(catalog.categories);
      setCollapsed(collapsedKeys(catalog.categories));
      setError(catalog.error ?? null);
    } catch (err: unknown) {
      setError(errorMessage(err));
    } finally {
      setRetrying(false);
    }
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
      className="flex min-h-0 shrink-0 border-r border-neutral-800 bg-neutral-950"
    >
      <nav className="min-w-0 flex-1 overflow-y-auto py-2">
        <div className="mb-1">
          <button
            type="button"
            onClick={() => {
              toggle('GitOps');
            }}
            className={sectionClass}
          >
            <span>{chevron(gitopsCollapsed)} GitOps</span>
          </button>
          {!gitopsCollapsed && (
            <div aria-label="GitOps views">
              <button
                type="button"
                onClick={onSelectRoles}
                className={resourceClass(view === 'flux-roles')}
              >
                Overview
              </button>
              <button
                type="button"
                onClick={onSelectGraph}
                className={resourceClass(view === 'gitops')}
              >
                Graph
              </button>
              <button
                type="button"
                onClick={onSelectList}
                className={resourceClass(view === 'flux-list')}
              >
                Resource list
              </button>
              <button
                type="button"
                onClick={onSelectOverview}
                className={resourceClass(view === 'flux-overview')}
              >
                Status tiles
              </button>
            </div>
          )}
        </div>
        {error !== null && (
          <div className="mx-2 mb-1 rounded border border-red-900 bg-red-950/40 px-2 py-1.5 text-[11px]">
            <div className="font-semibold text-red-400">Discovery failed</div>
            <div className="mt-0.5 break-words text-red-300">{error}</div>
            <button
              type="button"
              onClick={() => void retry()}
              disabled={retrying}
              className="mt-1.5 rounded border border-red-800 px-1.5 py-0.5 text-red-200 hover:bg-red-900 disabled:cursor-not-allowed disabled:text-red-500"
            >
              {retryLabel(retrying)}
            </button>
          </div>
        )}
        {error === null && categories.length === 0 && (
          <div className="px-3 py-1 text-[11px] text-neutral-400">
            No resource types discovered.
          </div>
        )}
        {categories.map((category) => {
          const isCollapsed = collapsed.has(category.name);
          return (
            <div key={category.name} className="mb-1">
              <button
                type="button"
                onClick={() => {
                  toggle(category.name);
                }}
                className={sectionClass}
              >
                <span>
                  {chevron(isCollapsed)} {category.name}
                </span>
                <span className="text-neutral-400">{category.resources.length}</span>
              </button>
              {!isCollapsed && !isNested(category.name) && (
                <div>
                  {category.resources.map((resource) => (
                    <button
                      key={descriptorKey(resource)}
                      type="button"
                      onClick={() => {
                        onSelect(resource);
                      }}
                      title={resource.kind}
                      className={resourceClass(isActive(activeResource, resource))}
                    >
                      {resource.kind}
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
                          onClick={() => {
                            toggle(key);
                          }}
                          title={group.name}
                          className="flex w-full items-center justify-between gap-1 px-5 py-1 text-left text-neutral-400 hover:bg-neutral-900 hover:text-neutral-200"
                        >
                          <span className="truncate">
                            {chevron(groupCollapsed)} {group.name}
                          </span>
                          <span className="shrink-0 text-neutral-400">
                            {group.resources.length}
                          </span>
                        </button>
                        {!groupCollapsed && (
                          <div>
                            {group.resources.map((resource) => (
                              <button
                                key={descriptorKey(resource)}
                                type="button"
                                onClick={() => {
                                  onSelect(resource);
                                }}
                                title={resource.kind}
                                className={resourceClass(isActive(activeResource, resource), true)}
                              >
                                {resource.kind}
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
        className="w-1 shrink-0 cursor-col-resize bg-neutral-500 hover:bg-neutral-300"
      />
    </div>
  );
}

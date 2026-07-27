import { useEffect, useState } from 'react';
import type { Category, ResourceDescriptor, View } from '../lib/types';
import { fetchResources } from '../lib/discovery';

interface SidebarProps {
  view: View;
  activeResource: ResourceDescriptor | null;
  onSelect: (descriptor: ResourceDescriptor) => void;
  onSelectGitops: () => void;
  onSelectFlux: () => void;
  onSelectTiles: () => void;
  onSelectResources: () => void;
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

function resourceClass(active: boolean): string {
  const base = 'block w-full px-6 py-1 text-left text-neutral-300 hover:bg-neutral-900';
  if (active) {
    return `${base} bg-neutral-800 text-neutral-100`;
  }
  return base;
}

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'discovery request failed';
}

const sectionClass =
  'flex w-full items-center justify-between px-3 py-1 text-[11px] font-semibold tracking-wide text-neutral-400 uppercase hover:text-neutral-200';

export default function Sidebar({
  view,
  activeResource,
  onSelect,
  onSelectGitops,
  onSelectFlux,
  onSelectTiles,
  onSelectResources,
}: SidebarProps) {
  const [categories, setCategories] = useState<Category[]>([]);
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let mounted = true;
    const load = async () => {
      try {
        const data = await fetchResources();
        if (mounted) {
          setCategories(data);
          setCollapsed(new Set(data.map((category) => category.name)));
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
  }, []);

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

  const gitopsCollapsed = collapsed.has('GitOps');

  return (
    <nav className="w-56 shrink-0 overflow-y-auto border-r border-neutral-800 bg-neutral-950 py-2">
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
          <div>
            <button
              type="button"
              onClick={onSelectGitops}
              className={resourceClass(view === 'gitops')}
            >
              Graph
            </button>
            <button type="button" onClick={onSelectFlux} className={resourceClass(view === 'flux')}>
              Flux
            </button>
            <button
              type="button"
              onClick={onSelectTiles}
              className={resourceClass(view === 'flux-tiles')}
            >
              Flux Dashboard
            </button>
            <button
              type="button"
              onClick={onSelectResources}
              className={resourceClass(view === 'flux-resources')}
            >
              Overview
            </button>
          </div>
        )}
      </div>
      {error !== null && <div className="px-3 py-1 text-[11px] text-red-400">{error}</div>}
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
              <span className="text-neutral-600">{category.resources.length}</span>
            </button>
            {!isCollapsed && (
              <div>
                {category.resources.map((resource) => (
                  <button
                    key={descriptorKey(resource)}
                    type="button"
                    onClick={() => {
                      onSelect(resource);
                    }}
                    className={resourceClass(isActive(activeResource, resource))}
                  >
                    {resource.kind}
                  </button>
                ))}
              </div>
            )}
          </div>
        );
      })}
    </nav>
  );
}

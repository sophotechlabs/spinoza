import { useEffect, useRef, useState } from 'react';
import type { Category, ResourceDescriptor, View } from '../lib/types';
import { fetchResources } from '../lib/discovery';
import { clusterItems, groupPaletteItems, paletteItems } from '../lib/palette';
import type { PaletteGroup, PaletteItem, PaletteOpen } from '../lib/palette';
import { SEARCH_DELAY_MS, searchObjects, worthSearching } from '../lib/search';
import type { SearchHit } from '../lib/types';
import { useRecents } from '../store/recents';
import { useClusterEpoch } from '../store/cluster';
import { useTrafficSupport } from '../store/traffic';
import { useTabStrip } from '../store/clusters';

interface CommandPaletteProps {
  open: boolean;
  onClose: () => void;
  onSelectView: (view: View) => void;
  onSelectResource: (descriptor: ResourceDescriptor) => void;
  onOpenObject: (found: PaletteOpen) => void;
}

const MAX_MATCHES = 60;

function rowClass(active: boolean): string {
  const base = 'flex w-full items-baseline gap-2 px-3 py-1.5 text-left';
  if (active) {
    return `${base} bg-surface-active text-fg-strong`;
  }
  return `${base} text-fg-soft hover:bg-surface-raised`;
}

function limitGroups(groups: PaletteGroup[], limit: number): PaletteGroup[] {
  const shown: PaletteGroup[] = [];
  let left = limit;
  for (const group of groups) {
    if (left === 0) {
      break;
    }
    const items = group.items.slice(0, left);
    shown.push({ ...group, items });
    left -= items.length;
  }
  return shown;
}

export default function CommandPalette({
  open,
  onClose,
  onSelectView,
  onSelectResource,
  onOpenObject,
}: CommandPaletteProps) {
  const ref = useRef<HTMLDialogElement | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const close = useRef(onClose);
  close.current = onClose;
  const recents = useRecents();
  const several = useTabStrip();
  const epoch = useClusterEpoch();
  const traffic = useTrafficSupport();
  const [categories, setCategories] = useState<Category[]>([]);
  const [query, setQuery] = useState('');
  const [cursor, setCursor] = useState(0);
  const [lastEpoch, setLastEpoch] = useState(epoch);
  if (epoch !== lastEpoch) {
    setLastEpoch(epoch);
    setQuery('');
  }
  const [hits, setHits] = useState<SearchHit[]>([]);
  const [partial, setPartial] = useState(false);
  const asked = useRef(0);

  useEffect(() => {
    const dialog = ref.current;
    if (open && dialog?.open === false) {
      dialog.showModal();
      inputRef.current?.focus();
      inputRef.current?.select();
    }
    if (!open && dialog?.open === true) {
      dialog.close();
    }
  }, [open]);

  useEffect(() => {
    function outside(event: MouseEvent) {
      if (event.target === ref.current) {
        close.current();
      }
    }
    document.addEventListener('click', outside);
    return () => {
      document.removeEventListener('click', outside);
    };
  }, []);

  useEffect(() => {
    if (!open) {
      setCursor(0);
      return;
    }
    let live = true;
    setCategories([]);
    fetchResources()
      .then((catalog) => {
        if (live) {
          setCategories(catalog.categories);
        }
      })
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [open, epoch]);

  useEffect(() => {
    asked.current += 1;
    const mine = asked.current;
    if (!open || !worthSearching(query)) {
      setHits([]);
      setPartial(false);
      return;
    }
    const timer = setTimeout(() => {
      searchObjects(query, several)
        .then((found) => {
          if (asked.current !== mine) {
            return;
          }
          setHits(found.hits);
          setPartial(found.truncated || Object.keys(found.errors ?? {}).length > 0);
        })
        .catch(() => {
          if (asked.current === mine) {
            setHits([]);
            setPartial(false);
          }
        });
    }, SEARCH_DELAY_MS);
    return () => {
      clearTimeout(timer);
    };
  }, [open, query, several]);

  const allGroups = groupPaletteItems(
    [...paletteItems(categories, recents, traffic.available), ...clusterItems(hits, categories)],
    query,
  );
  const groups = limitGroups(allGroups, MAX_MATCHES);
  const allMatches = allGroups.flatMap((group) => group.items);
  const matches = groups.flatMap((group) => group.items);
  let active = cursor;
  if (active < 0 || active >= matches.length) {
    active = 0;
  }

  function run(item: PaletteItem) {
    onClose();
    if (item.kind === 'view') {
      onSelectView(item.view);
      return;
    }
    if (item.kind === 'resource') {
      onSelectResource(item.descriptor);
      return;
    }
    let cluster: string | undefined;
    if (item.kind === 'object') {
      cluster = item.cluster;
    }
    onOpenObject({
      ref: item.ref,
      type: item.type,
      filter: query.trim(),
      cluster,
    });
  }

  function handleKeyDown(event: React.KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      if (matches.length === 0) {
        setCursor(0);
        return;
      }
      setCursor((value) => Math.min(value + 1, matches.length - 1));
      return;
    }
    if (event.key === 'ArrowUp') {
      event.preventDefault();
      setCursor((value) => Math.max(value - 1, 0));
      return;
    }
    if (event.key !== 'Enter') {
      return;
    }
    event.preventDefault();
    if (matches.length === 0) {
      return;
    }
    run(matches[active]);
  }

  return (
    <dialog
      ref={ref}
      aria-label="Command palette"
      onClose={onClose}
      className="backdrop:bg-black/50 mx-auto mt-24 w-[32rem] rounded border border-edge-strong bg-surface p-0 text-fg"
    >
      <div className="text-xs">
        <input
          ref={inputRef}
          type="text"
          onKeyDown={handleKeyDown}
          aria-label="Search resources, views and recent objects"
          placeholder="Search"
          value={query}
          onChange={(event) => {
            setQuery(event.target.value);
            setCursor(0);
          }}
          className="w-full border-b border-edge bg-surface px-3 py-2 text-fg placeholder:text-fg-muted focus:outline-none"
        />
        {matches.length === 0 && <p className="px-3 py-3 text-fg-muted">Nothing matches that.</p>}
        {partial && (
          <p className="border-b border-edge px-3 py-1 text-fg-muted">
            Some of the cluster could not be searched.
          </p>
        )}
        {matches.length < allMatches.length && (
          <p className="border-b border-edge px-3 py-1 text-fg-muted">
            Showing the first {MAX_MATCHES} matches. Narrow the search to see the rest.
          </p>
        )}
        <div className="max-h-80 overflow-y-auto py-1">
          {groups.map((group) => (
            <section key={group.id} aria-labelledby={`palette-group-${group.id}`}>
              <h2
                id={`palette-group-${group.id}`}
                className="px-3 pt-2 pb-1 text-[10px] font-semibold tracking-wide text-fg-muted uppercase"
              >
                {group.label}
              </h2>
              <ul>
                {group.items.map((item) => {
                  const index = matches.indexOf(item);
                  return (
                    <li key={item.id}>
                      <button
                        type="button"
                        aria-current={index === active}
                        onClick={() => {
                          run(item);
                        }}
                        onMouseEnter={() => {
                          setCursor(index);
                        }}
                        className={rowClass(index === active)}
                      >
                        <span className="truncate">{item.label}</span>
                        <span className="ml-auto shrink-0 truncate text-fg-muted">{item.hint}</span>
                      </button>
                    </li>
                  );
                })}
              </ul>
            </section>
          ))}
        </div>
      </div>
    </dialog>
  );
}

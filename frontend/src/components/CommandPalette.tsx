import { useEffect, useRef, useState } from 'react';
import type { Category, ResourceDescriptor, View } from '../lib/types';
import { fetchResources } from '../lib/discovery';
import { clusterItems, matchItems, paletteItems } from '../lib/palette';
import type { PaletteItem, PaletteOpen } from '../lib/palette';
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

function rowClass(active: boolean): string {
  const base = 'flex w-full items-baseline gap-2 px-3 py-1.5 text-left';
  if (active) {
    return `${base} bg-surface-active text-fg-strong`;
  }
  return `${base} text-fg-soft hover:bg-surface-raised`;
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
  }, [open]);

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

  const matches = [
    ...matchItems(paletteItems(categories, recents, traffic.available), query),
    ...clusterItems(hits, categories),
  ];
  let active = cursor;
  if (active >= matches.length) {
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
    onOpenObject({
      ref: item.ref,
      type: item.type,
      filter: query.trim(),
      cluster: item.cluster,
    });
  }

  function handleKeyDown(event: React.KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'ArrowDown') {
      event.preventDefault();
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
        <ul className="max-h-80 overflow-y-auto py-1">
          {matches.slice(0, 60).map((item, index) => (
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
          ))}
        </ul>
      </div>
    </dialog>
  );
}

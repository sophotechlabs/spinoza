import { useEffect, useRef, useState } from 'react';
import type { Category, ObjectRef, ResourceDescriptor, View } from '../lib/types';
import { fetchResources } from '../lib/discovery';
import { matchItems, paletteItems } from '../lib/palette';
import type { PaletteItem } from '../lib/palette';
import { useRecents } from '../store/recents';

interface CommandPaletteProps {
  open: boolean;
  onClose: () => void;
  onSelectView: (view: View) => void;
  onSelectResource: (descriptor: ResourceDescriptor) => void;
  onSelectObject: (ref: ObjectRef) => void;
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
  onSelectObject,
}: CommandPaletteProps) {
  const ref = useRef<HTMLDialogElement | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const recents = useRecents();
  const [categories, setCategories] = useState<Category[]>([]);
  const [query, setQuery] = useState('');
  const [cursor, setCursor] = useState(0);

  useEffect(() => {
    const dialog = ref.current;
    if (open && dialog?.open === false) {
      dialog.showModal();
      inputRef.current?.focus();
    }
    if (!open && dialog?.open === true) {
      dialog.close();
    }
  }, [open]);

  useEffect(() => {
    if (!open) {
      setQuery('');
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

  const matches = matchItems(paletteItems(categories, recents), query);
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
    onSelectObject(item.ref);
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
          placeholder="Search resources, views and recent objects…"
          value={query}
          onChange={(event) => {
            setQuery(event.target.value);
            setCursor(0);
          }}
          className="w-full border-b border-edge bg-surface px-3 py-2 text-fg placeholder:text-fg-muted focus:outline-none"
        />
        {matches.length === 0 && <p className="px-3 py-3 text-fg-muted">Nothing matches that.</p>}
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

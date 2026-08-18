import { useMemo, useState } from 'react';
import type { FilterField } from '../lib/filterChips';
import type { Row } from '../lib/types';
import { NAMESPACE_FIELD, fieldFor, labelOf, parseChip } from '../lib/filterChips';
import { suggest } from '../lib/filterSuggest';
import type { Suggestion } from '../lib/filterSuggest';
import { useChips, useFiltersStore } from '../store/filters';
import { ALL, useNamespaceStore } from '../store/namespace';
import { FILTER_INPUT_ID } from '../lib/hotkeys';

interface FilterBarProps {
  stateKey: string;
  fields: FilterField[];
  rows: Row[];
  text: string;
  onText: (value: string) => void;
}

interface Shown {
  id: string;
  label: string;
  value: string;
  drop: () => void;
}

const LIST_ID = 'resource-filter-suggestions';

function optionId(index: number): string {
  return `${LIST_ID}-${String(index)}`;
}

function placeholderFor(count: number): string {
  if (count > 0) {
    return 'Add a filter';
  }
  return 'Filter by name, or field:value';
}

function optionClass(active: boolean): string {
  const base = 'flex w-full items-baseline gap-2 px-2 py-1 text-left';
  if (active) {
    return `${base} bg-surface-active text-fg-strong`;
  }
  return `${base} text-fg-soft hover:bg-surface-raised`;
}

function activeOption(open: boolean, cursor: number): string | undefined {
  if (!open || cursor < 0) {
    return undefined;
  }
  return optionId(cursor);
}

export default function FilterBar({ stateKey, fields, rows, text, onText }: FilterBarProps) {
  const chips = useChips(stateKey);
  const add = useFiltersStore((state) => state.add);
  const removeAt = useFiltersStore((state) => state.removeAt);
  const namespace = useNamespaceStore((state) => state.namespace);
  const namespaces = useNamespaceStore((state) => state.names);
  const choose = useNamespaceStore((state) => state.choose);
  const [cursor, setCursor] = useState(-1);
  const [dismissed, setDismissed] = useState(false);

  const suggestions = useMemo(
    () => suggest(text, fields, rows, namespaces),
    [text, fields, rows, namespaces],
  );
  const open = !dismissed && suggestions.length > 0;

  const shown: Shown[] = [];
  if (fieldFor(fields, NAMESPACE_FIELD) !== null && namespace !== ALL) {
    shown.push({
      id: 'namespace',
      label: 'Namespace',
      value: namespace,
      drop: () => {
        choose(ALL);
      },
    });
  }
  chips.forEach((chip, index) => {
    shown.push({
      id: `${chip.field}:${chip.value}`,
      label: labelOf(chip, fields),
      value: chip.value,
      drop: () => {
        removeAt(stateKey, index);
      },
    });
  });

  function knownNamespace(value: string): boolean {
    if (namespaces.length === 0) {
      return true;
    }
    return namespaces.includes(value);
  }

  function commitText(wanted: string) {
    const chip = parseChip(wanted, fields);
    if (chip === null) {
      return;
    }
    if (chip.field === NAMESPACE_FIELD) {
      if (!knownNamespace(chip.value)) {
        return;
      }
      onText('');
      setCursor(-1);
      choose(chip.value);
      return;
    }
    onText('');
    setCursor(-1);
    add(stateKey, chip);
  }

  function accept(suggestion: Suggestion) {
    setCursor(-1);
    if (suggestion.kind === 'field') {
      onText(suggestion.text);
      return;
    }
    commitText(suggestion.text);
  }

  function change(value: string) {
    onText(value);
    setCursor(-1);
    setDismissed(false);
  }

  function moveDown(event: React.KeyboardEvent<HTMLInputElement>) {
    if (!open) {
      return;
    }
    event.preventDefault();
    setCursor(Math.min(cursor + 1, suggestions.length - 1));
  }

  function moveUp(event: React.KeyboardEvent<HTMLInputElement>) {
    if (!open) {
      return;
    }
    event.preventDefault();
    setCursor(Math.max(cursor - 1, -1));
  }

  function takeEnter(event: React.KeyboardEvent<HTMLInputElement>) {
    event.preventDefault();
    if (open && cursor >= 0) {
      accept(suggestions[cursor]);
      return;
    }
    commitText(text);
  }

  function takeTab(event: React.KeyboardEvent<HTMLInputElement>) {
    if (!open) {
      return;
    }
    event.preventDefault();
    accept(suggestions[Math.max(cursor, 0)]);
  }

  function takeEscape(event: React.KeyboardEvent<HTMLInputElement>) {
    if (!open) {
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    setDismissed(true);
    setCursor(-1);
  }

  function takeBackspace() {
    if (text !== '' || shown.length === 0) {
      return;
    }
    shown[shown.length - 1].drop();
  }

  function handleKeyDown(event: React.KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'ArrowDown') {
      moveDown(event);
      return;
    }
    if (event.key === 'ArrowUp') {
      moveUp(event);
      return;
    }
    if (event.key === 'Enter') {
      takeEnter(event);
      return;
    }
    if (event.key === 'Tab') {
      takeTab(event);
      return;
    }
    if (event.key === 'Escape') {
      takeEscape(event);
      return;
    }
    if (event.key === 'Backspace') {
      takeBackspace();
    }
  }

  return (
    <div className="relative">
      <div className="flex min-w-72 flex-wrap items-center gap-1 rounded border border-edge bg-surface-raised px-1.5 py-1 focus-within:border-edge-emphasis">
        {shown.map((chip) => (
          <span
            key={chip.id}
            className="flex max-w-56 items-center gap-1 rounded-sm border border-edge-strong bg-surface-active px-1.5 text-fg"
          >
            <span className="shrink-0 text-fg-muted">{chip.label}:</span>
            <span className="truncate">{chip.value}</span>
            <button
              type="button"
              aria-label={`Remove the ${chip.label} ${chip.value} filter`}
              onClick={chip.drop}
              className="shrink-0 cursor-pointer text-fg-muted hover:text-fg-strong"
            >
              ×
            </button>
          </span>
        ))}
        <input
          id={FILTER_INPUT_ID}
          type="search"
          role="combobox"
          aria-label="Filter"
          aria-expanded={open}
          aria-controls={LIST_ID}
          aria-activedescendant={activeOption(open, cursor)}
          aria-autocomplete="list"
          title="Type a name, or field:value. Enter keeps it as a chip."
          placeholder={placeholderFor(shown.length)}
          value={text}
          onChange={(event) => {
            change(event.target.value);
          }}
          onKeyDown={handleKeyDown}
          className="w-24 min-w-24 flex-1 bg-transparent text-fg placeholder:text-fg-muted focus:outline-none"
        />
      </div>
      {open && (
        <div
          id={LIST_ID}
          role="listbox"
          aria-label="Filter suggestions"
          className="absolute z-20 mt-1 flex max-h-72 w-full flex-col overflow-y-auto rounded border border-edge-strong bg-surface-raised py-1 shadow"
        >
          {suggestions.map((suggestion, index) => (
            <button
              key={suggestion.text}
              type="button"
              id={optionId(index)}
              role="option"
              aria-selected={index === cursor}
              onMouseDown={(event) => {
                event.preventDefault();
              }}
              onClick={() => {
                accept(suggestion);
              }}
              className={optionClass(index === cursor)}
            >
              <span className="truncate">{suggestion.label}</span>
              <span className="ml-auto shrink-0 text-fg-muted">{suggestion.hint}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

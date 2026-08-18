import type { FilterField } from '../lib/filterChips';
import { NAMESPACE_FIELD, fieldFor, labelOf, parseChip } from '../lib/filterChips';
import { useChips, useFiltersStore } from '../store/filters';
import { ALL, useNamespaceStore } from '../store/namespace';
import { FILTER_INPUT_ID } from '../lib/hotkeys';

interface FilterBarProps {
  stateKey: string;
  fields: FilterField[];
  text: string;
  onText: (value: string) => void;
}

interface Shown {
  id: string;
  label: string;
  value: string;
  drop: () => void;
}

function placeholderFor(count: number): string {
  if (count > 0) {
    return 'Add a filter';
  }
  return 'Filter by name, or field:value';
}

export default function FilterBar({ stateKey, fields, text, onText }: FilterBarProps) {
  const chips = useChips(stateKey);
  const add = useFiltersStore((state) => state.add);
  const removeAt = useFiltersStore((state) => state.removeAt);
  const namespace = useNamespaceStore((state) => state.namespace);
  const choose = useNamespaceStore((state) => state.choose);

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

  function commit() {
    const chip = parseChip(text, fields);
    if (chip === null) {
      return;
    }
    onText('');
    if (chip.field === NAMESPACE_FIELD) {
      choose(chip.value);
      return;
    }
    add(stateKey, chip);
  }

  function handleKeyDown(event: React.KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'Enter') {
      event.preventDefault();
      commit();
      return;
    }
    if (event.key !== 'Backspace') {
      return;
    }
    if (text !== '' || shown.length === 0) {
      return;
    }
    shown[shown.length - 1].drop();
  }

  return (
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
        aria-label="Filter"
        title="Type a name, or field:value. Enter keeps it as a chip."
        placeholder={placeholderFor(shown.length)}
        value={text}
        onChange={(event) => {
          onText(event.target.value);
        }}
        onKeyDown={handleKeyDown}
        className="w-24 min-w-24 flex-1 bg-transparent text-fg placeholder:text-fg-muted focus:outline-none"
      />
    </div>
  );
}

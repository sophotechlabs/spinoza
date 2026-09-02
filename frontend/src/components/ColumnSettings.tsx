import { useEffect, useState } from 'react';
import type { CustomColumn, ResourceDescriptor } from '../lib/types';
import { descriptorKey } from '../lib/discovery';
import { rememberCatalog, useCategories } from '../store/catalog';
import { fetchResources } from '../lib/discovery';
import { readColumns, writeColumns } from '../lib/settings';
import { useClusterEpoch } from '../store/cluster';

const MAX_COLUMNS = 8;

function everyKind(categories: { resources: ResourceDescriptor[] }[]): ResourceDescriptor[] {
  const out: ResourceDescriptor[] = [];
  for (const category of categories) {
    out.push(...category.resources);
  }
  out.sort((left, right) => left.kind.localeCompare(right.kind));
  return out;
}

function kindLabel(resource: ResourceDescriptor): string {
  if (resource.group === '') {
    return resource.kind;
  }
  return `${resource.kind} · ${resource.group}`;
}

export default function ColumnSettings() {
  const epoch = useClusterEpoch();
  const kinds = everyKind(useCategories());
  const [failed, setFailed] = useState('');
  const [held, setHeld] = useState<Record<string, CustomColumn[]>>(() => readColumns());
  const [kind, setKind] = useState('');
  const [name, setName] = useState('');
  const [path, setPath] = useState('');
  const [lastEpoch, setLastEpoch] = useState(epoch);

  if (epoch !== lastEpoch) {
    setLastEpoch(epoch);
    setFailed('');
  }

  const chosen =
    kind === '' ? (kinds[0] ?? null) : (kinds.find((one) => descriptorKey(one) === kind) ?? null);
  const key = chosen === null ? '' : descriptorKey(chosen);
  const columns = held[key] ?? [];

  useEffect(() => {
    if (kinds.length > 0) {
      return;
    }
    let live = true;
    fetchResources()
      .then((catalog) => {
        if (live) {
          rememberCatalog(catalog.categories);
        }
      })
      .catch((reason: unknown) => {
        if (!live) {
          return;
        }
        if (reason instanceof Error) {
          setFailed(reason.message);
          return;
        }
        setFailed('the discovery request failed');
      });
    return () => {
      live = false;
    };
  }, [kinds.length, epoch]);

  function save(next: Record<string, CustomColumn[]>) {
    setHeld(next);
    void writeColumns(next);
  }

  function ready(): boolean {
    return key !== '' && name.trim() !== '' && path.trim() !== '' && columns.length < MAX_COLUMNS;
  }

  function add() {
    save({ ...held, [key]: [...columns, { name: name.trim(), path: path.trim() }] });
    setName('');
    setPath('');
  }

  function remove(at: number) {
    save({ ...held, [key]: columns.filter((_, index) => index !== at) });
  }

  if (kinds.length === 0) {
    if (failed !== '') {
      return <p className="px-1 py-3 text-error">{failed}</p>;
    }
    return <p className="px-1 py-3 text-fg-muted">Reading what this cluster has…</p>;
  }

  return (
    <div className="px-1 py-3 text-xs">
      <label className="flex items-center justify-between gap-4 pb-3">
        <span className="text-fg">Kind</span>
        <select
          aria-label="Kind"
          value={key}
          onChange={(event) => {
            setKind(event.target.value);
          }}
          className="rounded border border-edge-strong bg-surface-raised px-2 py-0.5 text-fg"
        >
          {kinds.map((one) => (
            <option key={descriptorKey(one)} value={descriptorKey(one)}>
              {kindLabel(one)}
            </option>
          ))}
        </select>
      </label>

      {columns.length === 0 && (
        <p className="pb-3 text-fg-muted">
          No columns added. The table shows what {chosen?.kind ?? 'this kind'} comes with.
        </p>
      )}
      {columns.length > 0 && (
        <ul className="pb-3">
          {columns.map((one, at) => (
            <li
              key={`${one.name}/${one.path}`}
              className="flex items-center justify-between gap-3 border-b border-edge py-1.5 last:border-b-0"
            >
              <span className="text-fg">{one.name}</span>
              <code className="flex-1 truncate text-fg-muted">{one.path}</code>
              <button
                type="button"
                aria-label={`Remove ${one.name}`}
                onClick={() => {
                  remove(at);
                }}
                className="rounded border border-edge-strong px-2 py-0.5 text-fg hover:bg-surface-active"
              >
                Remove
              </button>
            </li>
          ))}
        </ul>
      )}

      <div className="flex items-center gap-2">
        <input
          aria-label="Column name"
          placeholder="App"
          value={name}
          onChange={(event) => {
            setName(event.target.value);
          }}
          className="w-32 rounded border border-edge-strong bg-surface-raised px-2 py-0.5 text-fg"
        />
        <input
          aria-label="Field path"
          placeholder=".metadata.labels.app"
          value={path}
          onChange={(event) => {
            setPath(event.target.value);
          }}
          className="flex-1 rounded border border-edge-strong bg-surface-raised px-2 py-0.5 text-fg"
        />
        <button
          type="button"
          disabled={!ready()}
          onClick={add}
          className="rounded border border-edge-strong px-2 py-0.5 text-fg hover:bg-surface-active disabled:opacity-50"
        >
          Add
        </button>
      </div>
      <p className="pt-2 text-fg-muted">
        A JSONPath into the object, the way kubectl takes it. A label or annotation whose key has
        dots in it needs them escaped:{' '}
        <code>{String.raw`.metadata.labels['app\.kubernetes\.io/name']`}</code>
      </p>
    </div>
  );
}

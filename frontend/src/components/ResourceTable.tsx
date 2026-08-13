import { useCallback, useMemo, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import {
  createColumnHelper,
  flexRender,
  functionalUpdate,
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table';
import type {
  ColumnDef,
  ColumnSizingInfoState,
  ColumnSizingState,
  RowSelectionState,
  SortDirection,
  SortingState,
  VisibilityState,
} from '@tanstack/react-table';
import { useVirtualizer } from '@tanstack/react-virtual';
import type { Metrics, ObjectRef, ResourceDescriptor, ResourceUsage, Row } from '../lib/types';
import {
  useSubColumns,
  useSubError,
  useSubLoaded,
  useSubNamespaced,
  useSubRows,
} from '../store/resources';
import { ratioColor, restartColor, statusColor } from '../lib/status';
import { useMetrics } from '../lib/metrics';
import { cpuFromMilli, memFromMi } from '../lib/units';
import { useElementWidth } from '../lib/useElementWidth';
import { useNow } from '../lib/useNow';
import { ago } from '../lib/time';
import { ALL_NAMESPACES, filterRows, namespacesOf } from '../lib/tableFilter';
import { FILTER_INPUT_ID } from '../lib/hotkeys';
import { columnLabel, readTableState, tableKey, writeTableState } from '../lib/tableState';
import ContainerSquares from './ContainerSquares';
import UsageBar from './UsageBar';
import StaleBanner from './StaleBanner';
import BulkBar from './BulkBar';
import CopyButton from './CopyButton';
import ColumnResizeHandle from './ColumnResizeHandle';

interface ResourceTableProps {
  active: ResourceDescriptor | null;
  subId: string;
  selected: Row | null;
  onSelect: (row: Row) => void;
}

const ROW_HEIGHT = 28;

const IDLE_SIZING: ColumnSizingInfoState = {
  startOffset: null,
  startSize: null,
  deltaOffset: null,
  deltaPercentage: null,
  isResizingColumn: false,
  columnSizingStart: [],
};
const FLEX_COLUMN_IDS = new Set(['name']);
const SELECT_COLUMN_ID = 'select';

function flexes(id: string, sizing: ColumnSizingState): boolean {
  if (!FLEX_COLUMN_IDS.has(id)) {
    return false;
  }
  return !Object.hasOwn(sizing, id);
}

function columnWidth(id: string, base: number, perFlex: number, sizing: ColumnSizingState): number {
  if (flexes(id, sizing)) {
    return base + perFlex;
  }
  return base;
}

function cellAt(row: Row, index: number): string {
  if (index >= row.cells.length) {
    return '';
  }
  return row.cells[index];
}

function renderDataCell(render: string | undefined, value: string, row: Row): ReactNode {
  if (render === 'containers') {
    return <ContainerSquares row={row} fallback={value} />;
  }
  if (render === 'ratio') {
    return <span className={ratioColor(value)}>{value}</span>;
  }
  if (render === 'restarts') {
    return <span className={restartColor(value)}>{value}</span>;
  }
  if (render === 'status') {
    return <span className={statusColor(value)}>{value}</span>;
  }
  return value;
}

const METRIC_KINDS = new Set(['Pod', 'Node']);

function metricUsage(kind: string, metrics: Metrics, row: Row): ResourceUsage | undefined {
  if (kind === 'Node') {
    return metrics.nodes[row.name];
  }
  return metrics.pods[`${row.namespace}/${row.name}`];
}

function nodeUsageCell(usage: ResourceUsage | undefined, memory: boolean): ReactNode {
  if (usage === undefined) {
    return <UsageBar percent={0} label="" />;
  }
  if (memory) {
    return <UsageBar percent={usage.memPercent} label={memFromMi(usage.memoryMi)} />;
  }
  return <UsageBar percent={usage.cpuPercent} label={cpuFromMilli(usage.cpuMilli)} />;
}

function podUsageCell(usage: ResourceUsage | undefined, memory: boolean): ReactNode {
  if (usage === undefined) {
    return <span className="text-fg-muted">—</span>;
  }
  if (memory) {
    return <span className="text-fg-muted">{memFromMi(usage.memoryMi)}</span>;
  }
  return <span className="text-fg-muted">{cpuFromMilli(usage.cpuMilli)}</span>;
}

function renderMetricCell(kind: string, metrics: Metrics, row: Row, memory: boolean): ReactNode {
  const usage = metricUsage(kind, metrics, row);
  if (kind === 'Node') {
    return nodeUsageCell(usage, memory);
  }
  return podUsageCell(usage, memory);
}

function sortIndicator(dir: false | SortDirection): string {
  if (dir === 'asc') {
    return ' ▲';
  }
  if (dir === 'desc') {
    return ' ▼';
  }
  return '';
}

function ariaSort(dir: false | SortDirection): 'ascending' | 'descending' | 'none' {
  if (dir === 'asc') {
    return 'ascending';
  }
  if (dir === 'desc') {
    return 'descending';
  }
  return 'none';
}

function rowClass(selected: boolean): string {
  const base = 'border-b border-edge';
  if (selected) {
    return `${base} bg-surface-active`;
  }
  return `${base} hover:bg-surface-raised`;
}

const columnHelper = createColumnHelper<Row>();

export default function ResourceTable({ active, subId, selected, onSelect }: ResourceTableProps) {
  const dataColumns = useSubColumns(subId);
  const namespaced = useSubNamespaced(subId);
  const rows = useSubRows(subId);
  const error = useSubError(subId);
  const loaded = useSubLoaded(subId);
  const now = useNow();
  const stateKey = tableKey(active);
  const [sorting, setSorting] = useState<SortingState>(() => readTableState(stateKey).sorting);
  const [visibility, setVisibility] = useState<VisibilityState>(
    () => readTableState(stateKey).visibility,
  );
  const [sizing, setSizing] = useState<ColumnSizingState>(() => readTableState(stateKey).sizing);
  const [sizingInfo, setSizingInfo] = useState<ColumnSizingInfoState>(IDLE_SIZING);
  const [selection, setSelection] = useState<RowSelectionState>({});
  const [query, setQuery] = useState('');
  const [namespace, setNamespace] = useState(ALL_NAMESPACES);
  const [lastResource, setLastResource] = useState(subId);
  if (subId !== lastResource) {
    setLastResource(subId);
    const next = readTableState(stateKey);
    setSorting(next.sorting);
    setVisibility(next.visibility);
    setSizing(next.sizing);
    setSelection({});
    setQuery('');
    setNamespace(ALL_NAMESPACES);
  }

  function changeSorting(next: SortingState) {
    setSorting(next);
    writeTableState(stateKey, { sorting: next, visibility, sizing });
  }

  function changeVisibility(next: VisibilityState) {
    setVisibility(next);
    writeTableState(stateKey, { sorting, visibility: next, sizing });
  }

  function changeSizing(next: ColumnSizingState) {
    setSizing(next);
    writeTableState(stateKey, { sorting, visibility, sizing: next });
  }
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const [scrollEl, setScrollEl] = useState<HTMLDivElement | null>(null);
  const setScroll = useCallback((node: HTMLDivElement | null) => {
    scrollRef.current = node;
    setScrollEl(node);
  }, []);

  let activeKind = '';
  if (active !== null) {
    activeKind = active.kind;
  }
  const wantMetrics = METRIC_KINDS.has(activeKind);
  const {
    data: metrics,
    error: metricsError,
    stale: metricsStale,
    reload: reloadMetrics,
  } = useMetrics(wantMetrics);

  const columns = useMemo<ColumnDef<Row, string>[]>(() => {
    const defs: ColumnDef<Row, string>[] = [];
    defs.push({
      id: SELECT_COLUMN_ID,
      size: 32,
      minSize: 32,
      enableSorting: false,
      enableResizing: false,
      enableHiding: false,
      header: ({ table }) => (
        <input
          type="checkbox"
          aria-label="Select every row"
          checked={table.getIsAllRowsSelected()}
          ref={(node) => {
            if (node !== null) {
              node.indeterminate = table.getIsSomeRowsSelected();
            }
          }}
          onChange={table.getToggleAllRowsSelectedHandler()}
        />
      ),
      cell: ({ row }) => (
        <input
          type="checkbox"
          aria-label={`Select ${row.original.name}`}
          checked={row.getIsSelected()}
          onChange={row.getToggleSelectedHandler()}
        />
      ),
    });
    defs.push(
      columnHelper.accessor('name', {
        header: 'Name',
        size: 240,
        minSize: 100,
        cell: (info) => (
          <span className="group flex items-baseline gap-1">
            <button
              type="button"
              onClick={() => {
                onSelect(info.row.original);
              }}
              className="min-w-0 cursor-pointer truncate text-left text-fg-strong hover:underline"
            >
              {info.getValue()}
            </button>
            <CopyButton what={info.getValue()} text={info.getValue()} quiet />
          </span>
        ),
      }),
    );
    if (namespaced) {
      defs.push(columnHelper.accessor('namespace', { header: 'Namespace', size: 150 }));
    }
    dataColumns.forEach((column, index) => {
      defs.push(
        columnHelper.accessor((row) => cellAt(row, index), {
          id: `cell-${index}`,
          header: column.name,
          size: 130,
          cell: (info) => renderDataCell(column.render, info.getValue(), info.row.original),
        }),
      );
    });
    if (wantMetrics && metrics !== null) {
      const sample = metrics;
      defs.push({
        id: 'cpu',
        header: 'CPU',
        size: 110,
        enableSorting: false,
        cell: (info) => renderMetricCell(activeKind, sample, info.row.original, false),
      });
      defs.push({
        id: 'memory',
        header: 'Memory',
        size: 110,
        enableSorting: false,
        cell: (info) => renderMetricCell(activeKind, sample, info.row.original, true),
      });
    }
    defs.push(
      columnHelper.accessor('createdAt', {
        id: 'age',
        header: 'Age',
        size: 72,
        minSize: 50,
        cell: (info) => ago(info.getValue(), now),
      }),
    );
    return defs;
  }, [dataColumns, namespaced, onSelect, activeKind, wantMetrics, metrics, now]);

  const namespaces = useMemo(() => namespacesOf(rows), [rows]);

  let activeNamespace = namespace;
  if (loaded && namespace !== ALL_NAMESPACES && !namespaces.includes(namespace)) {
    activeNamespace = ALL_NAMESPACES;
  }
  if (activeNamespace !== namespace) {
    setNamespace(activeNamespace);
  }

  const visibleRows = useMemo(
    () => filterRows(rows, query, activeNamespace),
    [rows, query, activeNamespace],
  );

  const table = useReactTable({
    data: visibleRows,
    columns,
    state: {
      sorting,
      columnVisibility: visibility,
      columnSizing: sizing,
      columnSizingInfo: sizingInfo,
      rowSelection: selection,
    },
    getRowId: (row) => row.uid,
    enableRowSelection: true,
    onRowSelectionChange: (updater) => {
      setSelection(functionalUpdate(updater, selection));
    },
    onSortingChange: (updater) => {
      changeSorting(functionalUpdate(updater, sorting));
    },
    onColumnVisibilityChange: (updater) => {
      changeVisibility(functionalUpdate(updater, visibility));
    },
    onColumnSizingChange: (updater) => {
      changeSizing(functionalUpdate(updater, sizing));
    },
    onColumnSizingInfoChange: (updater) => {
      setSizingInfo(functionalUpdate(updater, sizingInfo));
    },
    columnResizeMode: 'onChange',
    defaultColumn: { minSize: 60 },
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  });

  const tableRows = table.getRowModel().rows;
  const leafColumnCount = table.getVisibleLeafColumns().length;
  const containerWidth = useElementWidth(scrollEl);
  const totalSize = table.getTotalSize();
  const flexCount = table
    .getVisibleLeafColumns()
    .filter((column) => flexes(column.id, sizing)).length;
  const perFlex = Math.max(0, containerWidth - totalSize) / Math.max(1, flexCount);
  const tableWidth = Math.max(containerWidth, totalSize);

  const virtualizer = useVirtualizer({
    count: tableRows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 12,
  });

  const virtualItems = virtualizer.getVirtualItems();
  let paddingTop = 0;
  let paddingBottom = 0;
  if (virtualItems.length > 0) {
    paddingTop = virtualItems[0].start;
    paddingBottom = virtualizer.getTotalSize() - virtualItems[virtualItems.length - 1].end;
  }

  let selectedUid: string | null = null;
  if (selected !== null) {
    selectedUid = selected.uid;
  }

  function clearFilter() {
    setQuery('');
    setNamespace(ALL_NAMESPACES);
  }

  function clearSelection() {
    setSelection({});
  }

  const hideable = table.getAllLeafColumns().filter((column) => column.getCanHide());
  const chosen = table.getSelectedRowModel().rows;
  let targets: ObjectRef[] = [];
  if (active !== null) {
    targets = chosen.map((row) => ({
      group: active.group,
      version: active.version,
      resource: active.resource,
      namespace: row.original.namespace,
      name: row.original.name,
    }));
  }

  if (active === null) {
    return (
      <div className="flex h-full items-center justify-center text-xs text-fg-muted">
        Select a resource to view.
      </div>
    );
  }

  if (error !== null) {
    return (
      <div className="flex h-full items-start justify-center p-6 text-xs">
        <div className="max-w-2xl rounded border border-error-line bg-error-tint/40 px-3 py-2">
          <div className="font-semibold text-error">{active.kind} could not be loaded</div>
          <div className="mt-1 break-words text-error-strong">{error}</div>
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      {metricsStale && metricsError !== null && (
        <StaleBanner what="Metrics" message={metricsError} onRetry={reloadMetrics} />
      )}
      <div className="flex shrink-0 items-center gap-2 border-b border-edge bg-surface px-2 py-1.5 text-xs">
        <input
          id={FILTER_INPUT_ID}
          type="search"
          aria-label="Filter by name"
          placeholder="Filter by name…"
          value={query}
          onChange={(event) => {
            setQuery(event.target.value);
          }}
          className="w-56 rounded border border-edge bg-surface-raised px-2 py-1 text-fg placeholder:text-fg-muted focus:border-edge-emphasis"
        />
        {namespaced && namespaces.length > 0 && (
          <select
            aria-label="Namespace"
            value={activeNamespace}
            onChange={(event) => {
              setNamespace(event.target.value);
            }}
            className="rounded border border-edge bg-surface-raised px-1.5 py-1 text-fg focus:border-edge-emphasis"
          >
            <option value={ALL_NAMESPACES}>All namespaces</option>
            {namespaces.map((name) => (
              <option key={name} value={name}>
                {name}
              </option>
            ))}
          </select>
        )}
        <details className="relative">
          <summary className="cursor-pointer rounded border border-edge px-2 py-1 text-fg-soft hover:bg-surface-raised">
            Columns
          </summary>
          <div className="absolute z-20 mt-1 max-h-72 w-56 overflow-y-auto rounded border border-edge-strong bg-surface-raised p-2 shadow">
            {hideable.map((column) => (
              <label key={column.id} className="flex items-center gap-2 py-0.5 text-fg-soft">
                <input
                  type="checkbox"
                  checked={column.getIsVisible()}
                  onChange={column.getToggleVisibilityHandler()}
                />
                {columnLabel(column.columnDef.header, column.id)}
              </label>
            ))}
          </div>
        </details>
        <span className="ml-auto text-fg-muted">
          {visibleRows.length} of {rows.length}
        </span>
      </div>
      <BulkBar
        kind={active.kind}
        targets={targets}
        onDone={clearSelection}
        onClear={clearSelection}
      />
      <div ref={setScroll} className="min-h-0 flex-1 overflow-auto">
        <table
          className="table-fixed border-collapse text-left text-xs"
          style={{ width: `${tableWidth}px` }}
        >
          <thead className="sticky top-0 z-10 bg-surface-raised text-fg-muted">
            {table.getHeaderGroups().map((headerGroup) => (
              <tr key={headerGroup.id} className="border-b border-edge">
                {headerGroup.headers.map((header) => (
                  <th
                    key={header.id}
                    aria-sort={ariaSort(header.column.getIsSorted())}
                    className="relative px-2 py-1 font-medium"
                    style={{
                      width: `${columnWidth(header.column.id, header.getSize(), perFlex, sizing)}px`,
                    }}
                  >
                    {header.column.getCanSort() && (
                      <button
                        type="button"
                        onClick={header.column.getToggleSortingHandler()}
                        className="flex w-full cursor-pointer items-center truncate font-medium select-none hover:text-fg-strong"
                      >
                        {flexRender(header.column.columnDef.header, header.getContext())}
                        {sortIndicator(header.column.getIsSorted())}
                      </button>
                    )}
                    {!header.column.getCanSort() &&
                      flexRender(header.column.columnDef.header, header.getContext())}
                    {header.column.getCanResize() && (
                      <ColumnResizeHandle
                        column={columnLabel(header.column.columnDef.header, header.column.id)}
                        size={header.getSize()}
                        min={header.column.columnDef.minSize ?? 0}
                        onSize={(next) => {
                          table.setColumnSizing((old) => ({ ...old, [header.column.id]: next }));
                        }}
                        onReset={() => {
                          header.column.resetSize();
                        }}
                        onMouseDown={header.getResizeHandler()}
                        onTouchStart={header.getResizeHandler()}
                      />
                    )}
                  </th>
                ))}
              </tr>
            ))}
          </thead>
          <tbody>
            {paddingTop > 0 && (
              <tr aria-hidden="true">
                <td colSpan={leafColumnCount} style={{ height: `${paddingTop}px` }} />
              </tr>
            )}
            {virtualItems.map((virtualRow) => {
              const row = tableRows[virtualRow.index];
              return (
                <tr
                  key={row.id}
                  className={rowClass(row.original.uid === selectedUid)}
                  style={{ height: `${ROW_HEIGHT}px` }}
                >
                  {row.getVisibleCells().map((cell) => (
                    <td
                      key={cell.id}
                      className="truncate px-2 py-1 text-fg"
                      style={{
                        width: `${columnWidth(cell.column.id, cell.column.getSize(), perFlex, sizing)}px`,
                      }}
                    >
                      {flexRender(cell.column.columnDef.cell, cell.getContext())}
                    </td>
                  ))}
                </tr>
              );
            })}
            {paddingBottom > 0 && (
              <tr aria-hidden="true">
                <td colSpan={leafColumnCount} style={{ height: `${paddingBottom}px` }} />
              </tr>
            )}
          </tbody>
        </table>
        {!loaded && <p className="p-6 text-center text-xs text-fg-muted">Loading {active.kind}…</p>}
        {loaded && rows.length === 0 && (
          <p className="p-6 text-center text-xs text-fg-muted">
            This cluster has no {active.kind} objects.
          </p>
        )}
        {loaded && rows.length > 0 && visibleRows.length === 0 && (
          <div className="flex flex-col items-center gap-2 p-6 text-xs text-fg-muted">
            <span>Nothing matches the current filter.</span>
            <button
              type="button"
              onClick={clearFilter}
              className="rounded border border-edge-strong px-2 py-1 text-fg hover:bg-surface-active"
            >
              Clear filter
            </button>
          </div>
        )}
      </div>
    </div>
  );
}

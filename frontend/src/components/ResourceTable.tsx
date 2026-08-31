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
  Column as TanColumn,
  ColumnDef,
  ColumnSizingState,
  Row as TanRow,
  RowSelectionState,
  SortDirection,
  SortingState,
  VisibilityState,
} from '@tanstack/react-table';
import { useVirtualizer } from '@tanstack/react-virtual';
import type {
  Column,
  Metrics,
  ObjectRef,
  ResourceDescriptor,
  ResourceUsage,
  Row,
} from '../lib/types';
import {
  useSubColumns,
  useSubError,
  useSubLimit,
  useSubLoaded,
  useSubNamespaced,
  useSubRows,
  useSubTotal,
} from '../store/resources';
import { conditionColor, ratioColor, restartColor, statusColor } from '../lib/status';
import { useMetrics } from '../lib/metrics';
import { cpuFromMilli, cpuPair, memFromMi, memPair } from '../lib/units';
import { useElementWidth } from '../lib/useElementWidth';
import { useDismissMenu } from '../lib/useDismissMenu';
import { extraWidths, widthOf } from '../lib/columnFit';
import { useNow } from '../lib/useNow';
import { ago } from '../lib/time';
import { fieldsOf, filterRows, parseChip } from '../lib/filterChips';
import { scopedBy } from '../lib/catalog';
import { useChips, useFiltersStore } from '../store/filters';
import { opensRow } from '../lib/rowClick';
import {
  columnLabel,
  metricHeader,
  nextMetricSort,
  readTableState,
  tableKey,
  writeTableState,
} from '../lib/tableState';
import type { MetricBasis } from '../lib/tableState';
import FilterBar from './FilterBar';
import ContainerSquares from './ContainerSquares';
import UsageBar from './UsageBar';
import StaleBanner from './StaleBanner';
import BulkBar from './BulkBar';
import CopyButton from './CopyButton';
import ColumnResizeHandle from './ColumnResizeHandle';
import Loading from './Loading';

interface ResourceTableProps {
  active: ResourceDescriptor | null;
  subId: string;
  scope: boolean | null;
  selected: Row | null;
  onSelect: (row: Row) => void;
  onMore?: (limit: number) => void;
}

const ROW_HEIGHT = 28;

const SELECT_COLUMN_ID = 'select';

function cellAt(row: Row, index: number): string {
  if (index >= row.cells.length) {
    return '';
  }
  return row.cells[index];
}

function renderDataCell(column: Column, value: string, row: Row, now: number): ReactNode {
  const render = column.render;
  if (render === 'age') {
    return agoOrDash(value, now);
  }
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
  if (render === 'condition') {
    return renderCondition(column.name, value);
  }
  return value;
}

function renderCondition(name: string, value: string): ReactNode {
  if (!answersACondition(value)) {
    return value;
  }
  return <span className={conditionColor(name, value)}>{value}</span>;
}

function answersACondition(value: string): boolean {
  if (value === 'True') {
    return true;
  }
  return value === 'False';
}

function agoOrDash(value: string, now: number): string {
  if (value === '') {
    return '-';
  }
  return ago(value, now);
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
    return <UsageBar percent={0} label="" text="" />;
  }
  if (memory) {
    return (
      <UsageBar
        percent={usage.memPercent}
        label={`${String(usage.memPercent)}% of memory`}
        text={memPair(usage.memoryMi, usage.memAllocatableMi)}
      />
    );
  }
  return (
    <UsageBar
      percent={usage.cpuPercent}
      label={`${String(usage.cpuPercent)}% of cpu`}
      text={cpuPair(usage.cpuMilli, usage.cpuAllocatableMilli)}
    />
  );
}

function podUsageCell(usage: ResourceUsage | undefined, memory: boolean): ReactNode {
  if (usage === undefined) {
    return <span className="text-fg-muted">-</span>;
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

const UNMEASURED = -1;

const METRIC_COLUMNS = ['cpu', 'memory'];

function nodeTotal(usage: ResourceUsage, memory: boolean): number {
  if (memory) {
    return usage.memAllocatableMi;
  }
  return usage.cpuAllocatableMilli;
}

function metricValue(
  kind: string,
  metrics: Metrics,
  row: Row,
  memory: boolean,
  basis: MetricBasis,
): number {
  const usage = metricUsage(kind, metrics, row);
  if (usage === undefined) {
    return UNMEASURED;
  }
  if (kind !== 'Node') {
    if (memory) {
      return usage.memoryMi;
    }
    return usage.cpuMilli;
  }
  if (basis === 'total') {
    return nodeTotal(usage, memory);
  }
  if (memory) {
    return usage.memPercent;
  }
  return usage.cpuPercent;
}

function metricKey(kind: string, metrics: Metrics, row: Row, memory: boolean): string {
  return String(metricValue(kind, metrics, row, memory, 'used'));
}

function byMetric(
  kind: string,
  metrics: Metrics,
  memory: boolean,
  basis: MetricBasis,
): (left: TanRow<Row>, right: TanRow<Row>) => number {
  return (left, right) =>
    metricValue(kind, metrics, left.original, memory, basis) -
    metricValue(kind, metrics, right.original, memory, basis);
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
  const base = 'cursor-pointer border-b border-edge';
  if (selected) {
    return `${base} bg-surface-active`;
  }
  return `${base} hover:bg-surface-raised`;
}

const columnHelper = createColumnHelper<Row>();

const LOAD_STEP = 100;

function countLabel(
  visible: number,
  loaded: number,
  total: number,
  limit: number,
  filtered: boolean,
): string {
  if (limit === 0) {
    return `${String(visible)} of ${String(loaded)}`;
  }
  if (filtered) {
    return `newest ${String(loaded)} of ${String(total)} matching in the cluster`;
  }
  if (visible === loaded) {
    return `newest ${String(loaded)} of ${String(total)}`;
  }
  return `${String(visible)} of the newest ${String(loaded)} · ${String(total)} in the cluster`;
}

export default function ResourceTable({
  active,
  subId,
  scope,
  selected,
  onSelect,
  onMore,
}: ResourceTableProps) {
  const dataColumns = useSubColumns(subId);
  const namespaced = useSubNamespaced(subId);
  const error = useSubError(subId);
  const loaded = useSubLoaded(subId);
  const now = useNow();
  const stateKey = tableKey(active);
  const [sorting, setSorting] = useState<SortingState>(() => readTableState(stateKey).sorting);
  const total = useSubTotal(subId);
  const limit = useSubLimit(subId);
  const rows = useSubRows(subId, sorting.length === 0 && limit === 0);
  const [visibility, setVisibility] = useState<VisibilityState>(
    () => readTableState(stateKey).visibility,
  );
  const [sizing, setSizing] = useState<ColumnSizingState>(() => readTableState(stateKey).sizing);
  const [bases, setBases] = useState<Partial<Record<string, MetricBasis>>>(
    () => readTableState(stateKey).bases,
  );
  const [selection, setSelection] = useState<RowSelectionState>({});
  const [text, setText] = useState('');
  const [lastResource, setLastResource] = useState(subId);
  if (subId !== lastResource) {
    setLastResource(subId);
    const next = readTableState(stateKey);
    setSorting(next.sorting);
    setVisibility(next.visibility);
    setSizing(next.sizing);
    setBases(next.bases);
    setSelection({});
    setText('');
  }
  const chips = useChips(stateKey);
  const clearKind = useFiltersStore((state) => state.clearKind);
  const scoped = scopedBy(scope, namespaced);
  const fields = useMemo(() => fieldsOf(dataColumns, scoped), [dataColumns, scoped]);

  function changeSorting(next: SortingState) {
    setSorting(next);
    writeTableState(stateKey, { sorting: next, visibility, sizing, bases });
  }

  function changeVisibility(next: VisibilityState) {
    setVisibility(next);
    writeTableState(stateKey, { sorting, visibility: next, sizing, bases });
  }

  function cycleMetric(id: string) {
    const next = nextMetricSort(id, sorting, bases[id] ?? 'used');
    const nextBases = { ...bases, [id]: next.basis };
    setBases(nextBases);
    setSorting(next.sorting);
    writeTableState(stateKey, {
      sorting: next.sorting,
      visibility,
      sizing,
      bases: nextBases,
    });
  }

  function cyclesBothSides(column: TanColumn<Row>): boolean {
    return activeKind === 'Node' && METRIC_COLUMNS.includes(column.id);
  }

  function changeSizing(next: ColumnSizingState) {
    setSizing(next);
    writeTableState(stateKey, { sorting, visibility, sizing: next, bases });
  }
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const columnsRef = useRef<HTMLDetailsElement | null>(null);
  useDismissMenu(columnsRef);
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

  const sortedIds = useMemo(() => new Set(sorting.map((entry) => entry.id)), [sorting]);
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
          cell: (info) => renderDataCell(column, info.getValue(), info.row.original, now),
        }),
      );
    });
    if (wantMetrics && metrics !== null) {
      const sample = metrics;
      defs.push(
        columnHelper.accessor((row) => metricKey(activeKind, sample, row, false), {
          id: 'cpu',
          header: metricHeader('CPU', sortedIds.has('cpu'), bases.cpu ?? 'used'),
          size: 132,
          sortDescFirst: true,
          sortingFn: byMetric(activeKind, sample, false, bases.cpu ?? 'used'),
          cell: (info) => renderMetricCell(activeKind, sample, info.row.original, false),
        }),
      );
      defs.push(
        columnHelper.accessor((row) => metricKey(activeKind, sample, row, true), {
          id: 'memory',
          header: metricHeader('Memory', sortedIds.has('memory'), bases.memory ?? 'used'),
          size: 132,
          sortDescFirst: true,
          sortingFn: byMetric(activeKind, sample, true, bases.memory ?? 'used'),
          cell: (info) => renderMetricCell(activeKind, sample, info.row.original, true),
        }),
      );
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
  }, [dataColumns, namespaced, onSelect, activeKind, wantMetrics, metrics, now, bases, sortedIds]);

  const visibleRows = useMemo(() => {
    const draft = parseChip(text, fields);
    if (draft === null) {
      return filterRows(rows, chips, fields);
    }
    return filterRows(rows, [...chips, draft], fields);
  }, [rows, chips, text, fields]);

  const table = useReactTable({
    data: visibleRows,
    columns,
    state: {
      sorting,
      columnVisibility: visibility,
      columnSizing: sizing,
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
    columnResizeMode: 'onChange',
    defaultColumn: { minSize: 60 },
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  });

  const tableRows = table.getRowModel().rows;
  const leafColumns = table.getVisibleLeafColumns();
  const leafColumnCount = leafColumns.length;
  const containerWidth = useElementWidth(scrollEl);
  const totalSize = table.getTotalSize();
  const stretch = extraWidths(
    leafColumns.map((column) => ({
      id: column.id,
      size: column.getSize(),
      fixed: !column.getCanResize(),
      sized: Object.hasOwn(sizing, column.id),
    })),
    Math.max(0, containerWidth - totalSize),
  );
  const tableWidth = Math.max(containerWidth, totalSize);

  const virtualizer = useVirtualizer({
    count: tableRows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 12,
  });

  function openRow(row: Row, target: EventTarget | null) {
    if (!opensRow(target)) {
      return;
    }
    onSelect(row);
  }

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

  function loadMore() {
    if (onMore === undefined) {
      return;
    }
    onMore(limit + LOAD_STEP);
  }

  function clearFilter() {
    setText('');
    clearKind(stateKey);
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
        <FilterBar stateKey={stateKey} fields={fields} rows={rows} text={text} onText={setText} />
        <details ref={columnsRef} className="relative">
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
          {countLabel(visibleRows.length, rows.length, total, limit, chips.length > 0)}
        </span>
        {limit > 0 && total > rows.length && (
          <button
            type="button"
            onClick={() => {
              loadMore();
            }}
            className="rounded border border-edge px-2 py-1 text-fg-soft hover:bg-surface-raised"
          >
            Load {LOAD_STEP} more
          </button>
        )}
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
                      width: `${widthOf(header.column.id, header.getSize(), stretch)}px`,
                    }}
                  >
                    {header.column.getCanSort() && (
                      <button
                        type="button"
                        onClick={
                          cyclesBothSides(header.column)
                            ? () => {
                                cycleMetric(header.column.id);
                              }
                            : header.column.getToggleSortingHandler()
                        }
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
                        size={widthOf(header.column.id, header.getSize(), stretch)}
                        min={header.column.columnDef.minSize ?? 0}
                        onSize={(next) => {
                          table.setColumnSizing((old) => ({ ...old, [header.column.id]: next }));
                        }}
                        onReset={() => {
                          header.column.resetSize();
                        }}
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
                  onClick={(event) => {
                    openRow(row.original, event.target);
                  }}
                  onKeyDown={(event) => {
                    if (event.key !== 'Enter') {
                      return;
                    }
                    openRow(row.original, event.target);
                  }}
                >
                  {row.getVisibleCells().map((cell) => (
                    <td
                      key={cell.id}
                      className="truncate px-2 py-1 text-fg"
                      style={{
                        width: `${widthOf(cell.column.id, cell.column.getSize(), stretch)}px`,
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
        {!loaded && <Loading what={active.kind} />}
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

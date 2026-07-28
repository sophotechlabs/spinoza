import { useCallback, useMemo, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import {
  createColumnHelper,
  flexRender,
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table';
import type { ColumnDef, SortDirection, SortingState } from '@tanstack/react-table';
import { useVirtualizer } from '@tanstack/react-virtual';
import type { Metrics, ResourceDescriptor, ResourceUsage, Row } from '../lib/types';
import { useSubColumns, useSubNamespaced, useSubRows } from '../store/resources';
import { ratioColor, restartColor, statusColor } from '../lib/status';
import { formatCpu, formatMem, useMetrics } from '../lib/metrics';
import { useElementWidth } from '../lib/useElementWidth';
import { ALL_NAMESPACES, filterRows, namespacesOf } from '../lib/tableFilter';
import ContainerSquares from './ContainerSquares';
import UsageBar from './UsageBar';

interface ResourceTableProps {
  active: ResourceDescriptor | null;
  subId: string;
  selected: Row | null;
  onSelect: (row: Row) => void;
}

const ROW_HEIGHT = 28;
const FLEX_COLUMN_IDS = new Set(['name']);

function columnWidth(id: string, base: number, perFlex: number): number {
  if (FLEX_COLUMN_IDS.has(id)) {
    return base + perFlex;
  }
  return base;
}

function age(createdAt: string): string {
  const created = new Date(createdAt).getTime();
  if (Number.isNaN(created)) {
    return '';
  }
  let seconds = Math.floor((Date.now() - created) / 1000);
  if (seconds < 0) {
    seconds = 0;
  }
  if (seconds < 60) {
    return `${seconds}s`;
  }
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) {
    return `${minutes}m`;
  }
  const hours = Math.floor(minutes / 60);
  if (hours < 24) {
    return `${hours}h`;
  }
  const days = Math.floor(hours / 24);
  return `${days}d`;
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
    return <UsageBar percent={usage.memPercent} label={formatMem(usage.memoryMi)} />;
  }
  return <UsageBar percent={usage.cpuPercent} label={formatCpu(usage.cpuMilli)} />;
}

function podUsageCell(usage: ResourceUsage | undefined, memory: boolean): ReactNode {
  if (usage === undefined) {
    return <span className="text-neutral-600">—</span>;
  }
  if (memory) {
    return <span className="text-neutral-400">{formatMem(usage.memoryMi)}</span>;
  }
  return <span className="text-neutral-400">{formatCpu(usage.cpuMilli)}</span>;
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
  const base = 'border-b border-neutral-900 hover:bg-neutral-900';
  if (selected) {
    return `${base} bg-neutral-800`;
  }
  return base;
}

const columnHelper = createColumnHelper<Row>();

export default function ResourceTable({ active, subId, selected, onSelect }: ResourceTableProps) {
  const dataColumns = useSubColumns(subId);
  const namespaced = useSubNamespaced(subId);
  const rows = useSubRows(subId);
  const [sorting, setSorting] = useState<SortingState>([]);
  const [query, setQuery] = useState('');
  const [namespace, setNamespace] = useState(ALL_NAMESPACES);
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
  const metrics = useMetrics(wantMetrics);

  const columns = useMemo<ColumnDef<Row, string>[]>(() => {
    const defs: ColumnDef<Row, string>[] = [];
    defs.push(
      columnHelper.accessor('name', {
        header: 'Name',
        size: 240,
        minSize: 100,
        cell: (info) => (
          <button
            type="button"
            onClick={() => {
              onSelect(info.row.original);
            }}
            className="block w-full cursor-pointer truncate text-left text-neutral-100 hover:underline"
          >
            {info.getValue()}
          </button>
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
      const loaded = metrics;
      defs.push({
        id: 'cpu',
        header: 'CPU',
        size: 110,
        enableSorting: false,
        cell: (info) => renderMetricCell(activeKind, loaded, info.row.original, false),
      });
      defs.push({
        id: 'memory',
        header: 'Memory',
        size: 110,
        enableSorting: false,
        cell: (info) => renderMetricCell(activeKind, loaded, info.row.original, true),
      });
    }
    defs.push(
      columnHelper.accessor('createdAt', {
        id: 'age',
        header: 'Age',
        size: 72,
        minSize: 50,
        cell: (info) => age(info.getValue()),
      }),
    );
    return defs;
  }, [dataColumns, namespaced, onSelect, activeKind, wantMetrics, metrics]);

  const namespaces = useMemo(() => namespacesOf(rows), [rows]);
  const visibleRows = useMemo(() => filterRows(rows, query, namespace), [rows, query, namespace]);

  const table = useReactTable({
    data: visibleRows,
    columns,
    state: { sorting },
    onSortingChange: setSorting,
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
    .filter((column) => FLEX_COLUMN_IDS.has(column.id)).length;
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

  if (active === null) {
    return (
      <div className="flex h-full items-center justify-center text-xs text-neutral-600">
        Select a resource to view.
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex shrink-0 items-center gap-2 border-b border-neutral-800 bg-neutral-950 px-2 py-1.5 text-xs">
        <input
          type="search"
          aria-label="Filter by name"
          placeholder="Filter by name…"
          value={query}
          onChange={(event) => {
            setQuery(event.target.value);
          }}
          className="w-56 rounded border border-neutral-800 bg-neutral-900 px-2 py-1 text-neutral-200 placeholder:text-neutral-600 focus:border-neutral-600 focus:outline-none"
        />
        {namespaced && namespaces.length > 0 && (
          <select
            aria-label="Namespace"
            value={namespace}
            onChange={(event) => {
              setNamespace(event.target.value);
            }}
            className="rounded border border-neutral-800 bg-neutral-900 px-1.5 py-1 text-neutral-200 focus:border-neutral-600 focus:outline-none"
          >
            <option value={ALL_NAMESPACES}>All namespaces</option>
            {namespaces.map((name) => (
              <option key={name} value={name}>
                {name}
              </option>
            ))}
          </select>
        )}
        <span className="ml-auto text-neutral-500">
          {visibleRows.length} of {rows.length}
        </span>
      </div>
      <div ref={setScroll} className="min-h-0 flex-1 overflow-auto">
        <table
          className="table-fixed border-collapse text-left text-xs"
          style={{ width: `${tableWidth}px` }}
        >
          <thead className="sticky top-0 z-10 bg-neutral-900 text-neutral-400">
            {table.getHeaderGroups().map((headerGroup) => (
              <tr key={headerGroup.id} className="border-b border-neutral-800">
                {headerGroup.headers.map((header) => (
                  <th
                    key={header.id}
                    aria-sort={ariaSort(header.column.getIsSorted())}
                    className="relative px-2 py-1 font-medium"
                    style={{
                      width: `${columnWidth(header.column.id, header.getSize(), perFlex)}px`,
                    }}
                  >
                    <button
                      type="button"
                      onClick={header.column.getToggleSortingHandler()}
                      className="flex w-full cursor-pointer items-center truncate font-medium select-none hover:text-neutral-100"
                    >
                      {flexRender(header.column.columnDef.header, header.getContext())}
                      {sortIndicator(header.column.getIsSorted())}
                    </button>
                    <div
                      aria-hidden="true"
                      onMouseDown={header.getResizeHandler()}
                      onTouchStart={header.getResizeHandler()}
                      className="absolute top-0 right-0 h-full w-1 cursor-col-resize touch-none bg-neutral-600 opacity-0 select-none hover:opacity-100"
                    />
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
                      className="truncate px-2 py-1 text-neutral-200"
                      style={{
                        width: `${columnWidth(cell.column.id, cell.column.getSize(), perFlex)}px`,
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
      </div>
    </div>
  );
}

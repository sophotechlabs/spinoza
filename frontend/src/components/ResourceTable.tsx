import { useMemo, useRef, useState } from 'react';
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
import type { ResourceDescriptor, Row } from '../lib/types';
import { useSubColumns, useSubNamespaced, useSubRows } from '../store/resources';
import { ratioColor, restartColor } from '../lib/status';
import ContainerSquares from './ContainerSquares';

interface ResourceTableProps {
  active: ResourceDescriptor | null;
  subId: string;
  selected: Row | null;
  onSelect: (row: Row) => void;
}

const ROW_HEIGHT = 28;

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
  return value;
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
  const scrollRef = useRef<HTMLDivElement>(null);

  const columns = useMemo<ColumnDef<Row, string>[]>(() => {
    const defs: ColumnDef<Row, string>[] = [];
    defs.push(
      columnHelper.accessor('name', {
        header: 'Name',
        cell: (info) => (
          <button
            type="button"
            onClick={() => {
              onSelect(info.row.original);
            }}
            className="w-full cursor-pointer text-left text-neutral-100 hover:underline"
          >
            {info.getValue()}
          </button>
        ),
      }),
    );
    if (namespaced) {
      defs.push(columnHelper.accessor('namespace', { header: 'Namespace' }));
    }
    dataColumns.forEach((column, index) => {
      defs.push(
        columnHelper.accessor((row) => cellAt(row, index), {
          id: `cell-${index}`,
          header: column.name,
          cell: (info) => renderDataCell(column.render, info.getValue(), info.row.original),
        }),
      );
    });
    defs.push(
      columnHelper.accessor('createdAt', {
        id: 'age',
        header: 'Age',
        cell: (info) => age(info.getValue()),
      }),
    );
    return defs;
  }, [dataColumns, namespaced, onSelect]);

  const table = useReactTable({
    data: rows,
    columns,
    state: { sorting },
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  });

  const tableRows = table.getRowModel().rows;
  const leafColumnCount = table.getVisibleLeafColumns().length;

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
    <div ref={scrollRef} className="h-full overflow-auto">
      <table className="w-full border-collapse text-left text-xs">
        <thead className="sticky top-0 z-10 bg-neutral-900 text-neutral-400">
          {table.getHeaderGroups().map((headerGroup) => (
            <tr key={headerGroup.id} className="border-b border-neutral-800">
              {headerGroup.headers.map((header) => (
                <th
                  key={header.id}
                  aria-sort={ariaSort(header.column.getIsSorted())}
                  className="px-2 py-1 font-medium"
                >
                  <button
                    type="button"
                    onClick={header.column.getToggleSortingHandler()}
                    className="flex cursor-pointer items-center font-medium select-none hover:text-neutral-100"
                  >
                    {flexRender(header.column.columnDef.header, header.getContext())}
                    {sortIndicator(header.column.getIsSorted())}
                  </button>
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
                  <td key={cell.id} className="px-2 py-1 text-neutral-200">
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
  );
}

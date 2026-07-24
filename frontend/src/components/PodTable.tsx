import { useMemo, useState } from 'react';
import {
  createColumnHelper,
  flexRender,
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table';
import type { SortDirection, SortingState } from '@tanstack/react-table';
import type { PodRow } from '../lib/types';

interface PodTableProps {
  rows: PodRow[];
  selectedUid: string | null;
  onSelect: (pod: PodRow) => void;
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

function sortIndicator(dir: false | SortDirection): string {
  if (dir === 'asc') {
    return ' ▲';
  }
  if (dir === 'desc') {
    return ' ▼';
  }
  return '';
}

function rowClass(selected: boolean): string {
  const base = 'cursor-pointer border-b border-neutral-900 hover:bg-neutral-900';
  if (selected) {
    return `${base} bg-neutral-800`;
  }
  return base;
}

const columnHelper = createColumnHelper<PodRow>();

export default function PodTable({ rows, selectedUid, onSelect }: PodTableProps) {
  const [sorting, setSorting] = useState<SortingState>([]);

  const columns = useMemo(
    () => [
      columnHelper.accessor('name', { header: 'Name' }),
      columnHelper.accessor('namespace', { header: 'Namespace' }),
      columnHelper.accessor('phase', { header: 'Status' }),
      columnHelper.accessor('ready', { header: 'Ready' }),
      columnHelper.accessor('restarts', { header: 'Restarts' }),
      columnHelper.accessor('createdAt', {
        id: 'age',
        header: 'Age',
        cell: (info) => age(info.getValue()),
      }),
      columnHelper.accessor('node', { header: 'Node' }),
    ],
    [],
  );

  const table = useReactTable({
    data: rows,
    columns,
    state: { sorting },
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  });

  return (
    <table className="w-full border-collapse text-left text-xs">
      <thead className="sticky top-0 bg-neutral-900 text-neutral-400">
        {table.getHeaderGroups().map((headerGroup) => (
          <tr key={headerGroup.id} className="border-b border-neutral-800">
            {headerGroup.headers.map((header) => (
              <th
                key={header.id}
                onClick={header.column.getToggleSortingHandler()}
                className="cursor-pointer select-none px-2 py-1 font-medium hover:text-neutral-100"
              >
                {flexRender(header.column.columnDef.header, header.getContext())}
                {sortIndicator(header.column.getIsSorted())}
              </th>
            ))}
          </tr>
        ))}
      </thead>
      <tbody>
        {table.getRowModel().rows.map((row) => (
          <tr
            key={row.id}
            onClick={() => onSelect(row.original)}
            className={rowClass(row.original.uid === selectedUid)}
          >
            {row.getVisibleCells().map((cell) => (
              <td key={cell.id} className="px-2 py-1 text-neutral-200">
                {flexRender(cell.column.columnDef.cell, cell.getContext())}
              </td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  );
}

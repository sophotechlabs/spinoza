import { Fragment, useState } from 'react';
import {
  createColumnHelper,
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table';
import type { FluxResource } from '../lib/types';
import { useFlux } from '../lib/flux';
import {
  created,
  latestColor,
  latestTitle,
  statusDot,
  statusLabel,
  statusText,
} from '../lib/fluxStatus';
import { useElementWidth } from '../lib/useElementWidth';

const EMPTY: FluxResource[] = [];
const FLEX_COLUMN_IDS = new Set(['name', 'revision']);

function columnWidth(id: string, base: number, perFlex: number): number {
  if (FLEX_COLUMN_IDS.has(id)) {
    return base + perFlex;
  }
  return base;
}

const columnHelper = createColumnHelper<FluxResource>();

const COLUMNS = [
  columnHelper.display({ id: 'kind', header: 'Kind', size: 120 }),
  columnHelper.display({ id: 'name', header: 'Name', size: 180 }),
  columnHelper.display({ id: 'namespace', header: 'Namespace', size: 130 }),
  columnHelper.display({ id: 'status', header: 'Status', size: 110 }),
  columnHelper.display({ id: 'revision', header: 'Revision', size: 260 }),
  columnHelper.display({ id: 'latest', header: 'Latest', size: 110 }),
  columnHelper.display({ id: 'source', header: 'Source', size: 180 }),
  columnHelper.display({ id: 'created', header: 'Created', size: 90 }),
];

function rowKey(resource: FluxResource): string {
  return `${resource.kind}/${resource.namespace}/${resource.name}`;
}

function ResourceRow({
  resource,
  onSelect,
}: {
  resource: FluxResource;
  onSelect: (resource: FluxResource) => void;
}) {
  function handleSelect() {
    onSelect(resource);
  }

  return (
    <tr className="border-t border-neutral-900 hover:bg-neutral-900">
      <td className="truncate px-2 py-1 text-neutral-400">{resource.kind}</td>
      <td className="truncate px-2 py-1 text-neutral-100">
        <button
          type="button"
          onClick={handleSelect}
          className="max-w-full truncate hover:underline"
        >
          {resource.name}
        </button>
      </td>
      <td className="truncate px-2 py-1 text-neutral-400">{resource.namespace}</td>
      <td className="truncate px-2 py-1" title={resource.message}>
        <span className={`inline-flex items-center gap-1.5 ${statusText(resource)}`}>
          <span className={`h-2 w-2 rounded-full ${statusDot(resource)}`} />
          {statusLabel(resource)}
        </span>
      </td>
      <td className="truncate px-2 py-1 text-neutral-400" title={resource.revision}>
        {resource.revision}
      </td>
      <td className={`truncate px-2 py-1 ${latestColor(resource)}`} title={latestTitle(resource)}>
        {resource.latest}
      </td>
      <td className="truncate px-2 py-1 text-neutral-400">{resource.source}</td>
      <td className="truncate px-2 py-1 text-neutral-500">{created(resource.createdAt)}</td>
    </tr>
  );
}

interface FluxListProps {
  onSelect: (resource: FluxResource) => void;
}

export default function FluxList({ onSelect }: FluxListProps) {
  const { data, error } = useFlux();
  const [scrollEl, setScrollEl] = useState<HTMLDivElement | null>(null);
  const containerWidth = useElementWidth(scrollEl);

  const table = useReactTable({
    data: EMPTY,
    columns: COLUMNS,
    columnResizeMode: 'onChange',
    defaultColumn: { minSize: 60 },
    getCoreRowModel: getCoreRowModel(),
  });

  if (data === null) {
    if (error !== null) {
      return (
        <div className="flex h-full items-center justify-center text-xs text-red-400">{error}</div>
      );
    }
    return (
      <div className="flex h-full items-center justify-center text-xs text-neutral-600">
        Loading Flux resources…
      </div>
    );
  }

  if (data.groups.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-xs text-neutral-600">
        No Flux resources found.
      </div>
    );
  }

  const columnCount = COLUMNS.length;
  const totalSize = table.getTotalSize();
  const flexCount = table
    .getVisibleLeafColumns()
    .filter((column) => FLEX_COLUMN_IDS.has(column.id)).length;
  const perFlex = Math.max(0, containerWidth - totalSize) / Math.max(1, flexCount);
  const tableWidth = Math.max(containerWidth, totalSize);

  return (
    <div ref={setScrollEl} className="h-full overflow-auto">
      <table
        className="table-fixed border-collapse text-left text-xs whitespace-nowrap"
        style={{ width: `${tableWidth}px` }}
      >
        <colgroup>
          {table.getVisibleLeafColumns().map((column) => (
            <col
              key={column.id}
              style={{ width: `${columnWidth(column.id, column.getSize(), perFlex)}px` }}
            />
          ))}
        </colgroup>
        <thead className="sticky top-0 z-10 bg-neutral-950 text-neutral-500">
          <tr className="border-b border-neutral-800">
            {table.getFlatHeaders().map((header) => (
              <th
                key={header.id}
                className="relative px-2 py-1 font-normal"
                style={{ width: `${columnWidth(header.column.id, header.getSize(), perFlex)}px` }}
              >
                {flexRender(header.column.columnDef.header, header.getContext())}
                <div
                  aria-hidden="true"
                  onMouseDown={header.getResizeHandler()}
                  onTouchStart={header.getResizeHandler()}
                  className="absolute top-0 right-0 h-full w-1 cursor-col-resize touch-none bg-neutral-600 opacity-0 select-none hover:opacity-100"
                />
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {data.groups.map((group) => (
            <Fragment key={group.name}>
              <tr>
                <td
                  colSpan={columnCount}
                  className="bg-neutral-900/50 px-2 pt-3 pb-1 text-xs font-semibold tracking-wide text-neutral-300 uppercase"
                >
                  {group.name}
                  <span className="ml-2 text-[11px] font-normal text-neutral-600">
                    {group.ready}/{group.total} ready
                  </span>
                </td>
              </tr>
              {group.resources.map((resource) => (
                <ResourceRow key={rowKey(resource)} resource={resource} onSelect={onSelect} />
              ))}
            </Fragment>
          ))}
        </tbody>
      </table>
    </div>
  );
}

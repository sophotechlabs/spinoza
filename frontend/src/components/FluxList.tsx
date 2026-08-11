import { Fragment, useState } from 'react';
import type { ReactNode } from 'react';
import {
  createColumnHelper,
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table';
import type { FluxResource } from '../lib/types';
import { useFlux } from '../lib/flux';
import { groupSummary } from '../lib/readiness';
import {
  created,
  latestColor,
  latestNote,
  latestTitle,
  statusDot,
  statusLabel,
  statusText,
} from '../lib/fluxStatus';
import { useElementWidth } from '../lib/useElementWidth';
import { columnLabel } from '../lib/tableState';
import LoadWarning from './LoadWarning';
import LoadFailure from './LoadFailure';
import StaleBanner from './StaleBanner';
import ColumnResizeHandle from './ColumnResizeHandle';

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
    <tr className="border-t border-edge hover:bg-surface-raised">
      <td className="truncate px-2 py-1 text-fg-muted">{resource.kind}</td>
      <td className="truncate px-2 py-1 text-fg-strong">
        <button
          type="button"
          onClick={handleSelect}
          className="max-w-full truncate hover:underline"
        >
          {resource.name}
        </button>
      </td>
      <td className="truncate px-2 py-1 text-fg-muted">{resource.namespace}</td>
      <td className="truncate px-2 py-1" title={resource.message}>
        <span className={`inline-flex items-center gap-1.5 ${statusText(resource)}`}>
          <span className={`h-2 w-2 rounded-full ${statusDot(resource)}`} />
          {statusLabel(resource)}
        </span>
      </td>
      <td className="truncate px-2 py-1 text-fg-muted" title={resource.revision}>
        {resource.revision}
      </td>
      <td className={`truncate px-2 py-1 ${latestColor(resource)}`} title={latestTitle(resource)}>
        {resource.latest}
        <span className="sr-only"> {latestNote(resource)}</span>
      </td>
      <td className="truncate px-2 py-1 text-fg-muted">{resource.source}</td>
      <td className="truncate px-2 py-1 text-fg-muted">{created(resource.createdAt)}</td>
    </tr>
  );
}

interface FluxListProps {
  onSelect: (resource: FluxResource) => void;
}

export default function FluxList({ onSelect }: FluxListProps) {
  const { data, error, reload } = useFlux();
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
        <div className="flex h-full items-center justify-center text-xs text-error">{error}</div>
      );
    }
    return (
      <div className="flex h-full items-center justify-center text-xs text-fg-muted">
        Loading Flux resources…
      </div>
    );
  }

  let notice: ReactNode = null;
  if (error !== null) {
    notice = <StaleBanner what="Flux resources" message={error} onRetry={reload} />;
  }

  if (data.groups.length === 0) {
    if (data.error !== undefined) {
      return <LoadFailure what="Flux resources" message={data.error} />;
    }
    return (
      <div className="flex h-full min-h-0 flex-col">
        {notice}
        <div className="flex flex-1 items-center justify-center text-xs text-fg-muted">
          No Flux resources found.
        </div>
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
    <div className="flex h-full min-h-0 flex-col">
      {notice}
      {data.error !== undefined && <LoadWarning message={data.error} />}
      <div ref={setScrollEl} className="min-h-0 flex-1 overflow-auto">
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
          <thead className="sticky top-0 z-10 bg-surface-raised text-fg-muted">
            <tr className="border-b border-edge">
              {table.getFlatHeaders().map((header) => (
                <th
                  key={header.id}
                  className="relative px-2 py-1 font-medium"
                  style={{ width: `${columnWidth(header.column.id, header.getSize(), perFlex)}px` }}
                >
                  {flexRender(header.column.columnDef.header, header.getContext())}
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
                    className="bg-surface-raised/50 px-2 pt-3 pb-1 text-xs font-semibold tracking-wide text-fg-soft uppercase"
                  >
                    {group.name}
                    <span className="ml-2 text-[11px] font-normal text-fg-muted">
                      {groupSummary(group)}
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
    </div>
  );
}

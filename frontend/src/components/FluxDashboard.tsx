import { Fragment, useEffect, useState } from 'react';
import {
  createColumnHelper,
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table';
import type { FluxDashboard as FluxDashboardData, FluxResource } from '../lib/types';
import { fetchFlux } from '../lib/flux';

const POLL_INTERVAL_MS = 5000;
const EMPTY: FluxResource[] = [];

const columnHelper = createColumnHelper<FluxResource>();

const COLUMNS = [
  columnHelper.display({ id: 'kind', header: 'Kind', size: 120 }),
  columnHelper.display({ id: 'name', header: 'Name', size: 180 }),
  columnHelper.display({ id: 'namespace', header: 'Namespace', size: 130 }),
  columnHelper.display({ id: 'status', header: 'Status', size: 110 }),
  columnHelper.display({ id: 'revision', header: 'Revision', size: 300 }),
  columnHelper.display({ id: 'source', header: 'Source', size: 180 }),
  columnHelper.display({ id: 'created', header: 'Created', size: 90 }),
];

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'flux request failed';
}

function readyDot(ready: string): string {
  if (ready === 'True') {
    return 'bg-green-500';
  }
  if (ready === 'False') {
    return 'bg-red-500';
  }
  return 'bg-neutral-500';
}

function readyText(ready: string): string {
  if (ready === 'True') {
    return 'text-green-400';
  }
  if (ready === 'False') {
    return 'text-red-400';
  }
  return 'text-neutral-500';
}

function readyLabel(ready: string): string {
  if (ready === 'True') {
    return 'Ready';
  }
  if (ready === 'False') {
    return 'Not ready';
  }
  return 'Unknown';
}

function statusDot(resource: FluxResource): string {
  if (resource.suspended) {
    return 'bg-amber-500';
  }
  return readyDot(resource.ready);
}

function statusText(resource: FluxResource): string {
  if (resource.suspended) {
    return 'text-amber-400';
  }
  return readyText(resource.ready);
}

function statusLabel(resource: FluxResource): string {
  if (resource.suspended) {
    return 'Suspended';
  }
  return readyLabel(resource.ready);
}

function created(createdAt: string): string {
  if (createdAt === '') {
    return '';
  }
  return createdAt.slice(0, 10);
}

function rowKey(resource: FluxResource): string {
  return `${resource.kind}/${resource.namespace}/${resource.name}`;
}

function ResourceRow({ resource }: { resource: FluxResource }) {
  return (
    <tr className="border-t border-neutral-900 hover:bg-neutral-900">
      <td className="truncate px-2 py-1 text-neutral-400">{resource.kind}</td>
      <td className="truncate px-2 py-1 text-neutral-100">{resource.name}</td>
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
      <td className="truncate px-2 py-1 text-neutral-400">{resource.source}</td>
      <td className="truncate px-2 py-1 text-neutral-500">{created(resource.createdAt)}</td>
    </tr>
  );
}

export default function FluxDashboard() {
  const [data, setData] = useState<FluxDashboardData | null>(null);
  const [error, setError] = useState<string | null>(null);

  const table = useReactTable({
    data: EMPTY,
    columns: COLUMNS,
    columnResizeMode: 'onChange',
    defaultColumn: { minSize: 60 },
    getCoreRowModel: getCoreRowModel(),
  });

  useEffect(() => {
    let mounted = true;
    const load = async () => {
      try {
        const dash = await fetchFlux();
        if (mounted) {
          setData(dash);
          setError(null);
        }
      } catch (err: unknown) {
        if (mounted) {
          setError(errorMessage(err));
        }
      }
    };
    void load();
    const timer = setInterval(() => {
      void load();
    }, POLL_INTERVAL_MS);
    return () => {
      mounted = false;
      clearInterval(timer);
    };
  }, []);

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

  return (
    <div className="h-full overflow-auto">
      <table
        className="table-fixed border-collapse text-left text-xs whitespace-nowrap"
        style={{ width: `${table.getTotalSize()}px` }}
      >
        <colgroup>
          {table.getVisibleLeafColumns().map((column) => (
            <col key={column.id} style={{ width: `${column.getSize()}px` }} />
          ))}
        </colgroup>
        <thead className="sticky top-0 z-10 bg-neutral-950 text-neutral-500">
          <tr className="border-b border-neutral-800">
            {table.getFlatHeaders().map((header) => (
              <th
                key={header.id}
                className="relative px-2 py-1 font-normal"
                style={{ width: `${header.getSize()}px` }}
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
                <ResourceRow key={rowKey(resource)} resource={resource} />
              ))}
            </Fragment>
          ))}
        </tbody>
      </table>
    </div>
  );
}

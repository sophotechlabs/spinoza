import { useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import type { FluxDashboard as FluxDashboardData, FluxGroup, FluxResource } from '../lib/types';
import { fetchFlux } from '../lib/flux';

const POLL_INTERVAL_MS = 5000;

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

function suspendedBadge(suspended: boolean): ReactNode {
  if (!suspended) {
    return null;
  }
  return (
    <span className="rounded bg-amber-950 px-1.5 py-0.5 text-[10px] text-amber-300">Suspended</span>
  );
}

function created(createdAt: string): string {
  if (createdAt === '') {
    return '';
  }
  return createdAt.slice(0, 10);
}

const HEADERS = [
  'Kind',
  'Name',
  'Namespace',
  'Ready',
  'Suspended',
  'Revision',
  'Source',
  'Created',
];

function ResourceRow({ resource }: { resource: FluxResource }) {
  return (
    <tr className="border-t border-neutral-900 hover:bg-neutral-900">
      <td className="px-2 py-1 text-neutral-400">{resource.kind}</td>
      <td className="px-2 py-1 text-neutral-100">{resource.name}</td>
      <td className="px-2 py-1 text-neutral-400">{resource.namespace}</td>
      <td className="px-2 py-1">
        <span className={`inline-flex items-center gap-1.5 ${readyText(resource.ready)}`}>
          <span className={`h-2 w-2 rounded-full ${readyDot(resource.ready)}`} />
          {readyLabel(resource.ready)}
        </span>
      </td>
      <td className="px-2 py-1">{suspendedBadge(resource.suspended)}</td>
      <td className="px-2 py-1 text-neutral-400">{resource.revision}</td>
      <td className="px-2 py-1 text-neutral-400">{resource.source}</td>
      <td className="px-2 py-1 text-neutral-500">{created(resource.createdAt)}</td>
    </tr>
  );
}

function GroupSection({ group }: { group: FluxGroup }) {
  return (
    <section className="mb-5">
      <h2 className="mb-1 flex items-center gap-2 px-2 text-xs font-semibold tracking-wide text-neutral-300 uppercase">
        {group.name}
        <span className="text-[11px] font-normal text-neutral-600">
          {group.ready}/{group.total} ready
        </span>
      </h2>
      <div className="overflow-x-auto">
        <table className="w-full border-collapse text-left text-xs">
          <thead>
            <tr className="text-neutral-500">
              {HEADERS.map((header) => (
                <th key={header} className="px-2 py-1 font-normal">
                  {header}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {group.resources.map((resource) => (
              <ResourceRow
                key={`${resource.kind}/${resource.namespace}/${resource.name}`}
                resource={resource}
              />
            ))}
          </tbody>
        </table>
      </div>
      {group.resources.map((resource) => (
        <Message
          key={`${resource.kind}/${resource.namespace}/${resource.name}`}
          resource={resource}
        />
      ))}
    </section>
  );
}

function Message({ resource }: { resource: FluxResource }) {
  if (resource.message === '') {
    return null;
  }
  return (
    <p className="px-2 pt-1 text-[11px] text-neutral-600">
      <span className="text-neutral-500">
        {resource.kind}/{resource.name}:
      </span>{' '}
      {resource.message}
    </p>
  );
}

export default function FluxDashboard() {
  const [data, setData] = useState<FluxDashboardData | null>(null);
  const [error, setError] = useState<string | null>(null);

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

  return (
    <div className="h-full overflow-y-auto p-3">
      {data.groups.map((group) => (
        <GroupSection key={group.name} group={group} />
      ))}
    </div>
  );
}

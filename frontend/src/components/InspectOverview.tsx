import type { Condition, ContainerState, ObjectDetail } from '../lib/types';
import { containerColor } from '../lib/status';

interface InspectOverviewProps {
  detail: ObjectDetail;
  containers?: ContainerState[];
}

function conditionColor(condition: Condition): string {
  if (condition.status === 'True') {
    return 'text-green-400';
  }
  if (condition.status === 'False') {
    return 'text-red-400';
  }
  return 'text-neutral-400';
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="border-b border-neutral-800 px-4 py-3">
      <h3 className="mb-2 text-[11px] font-semibold tracking-wide text-neutral-400 uppercase">
        {title}
      </h3>
      {children}
    </section>
  );
}

function Pairs({ pairs }: { pairs: [string, string][] }) {
  return (
    <dl className="grid grid-cols-[minmax(0,9rem)_1fr] gap-x-3 gap-y-1">
      {pairs.map(([label, value]) => (
        <div key={label} className="contents">
          <dt className="truncate text-neutral-400">{label}</dt>
          <dd className="break-all text-neutral-200">{value}</dd>
        </div>
      ))}
    </dl>
  );
}

function entries(map: Record<string, string> | undefined): [string, string][] {
  if (map === undefined) {
    return [];
  }
  return Object.entries(map).sort((a, b) => a[0].localeCompare(b[0]));
}

export default function InspectOverview({ detail, containers }: InspectOverviewProps) {
  const labels = entries(detail.labels);
  const annotations = entries(detail.annotations);
  const owners = detail.owners ?? [];
  const conditions = detail.conditions ?? [];
  const runtimeContainers = containers ?? [];

  return (
    <div className="overflow-y-auto text-xs">
      <Section title="Metadata">
        <Pairs
          pairs={[
            ['API version', detail.apiVersion],
            ['Kind', detail.kind],
            ['Name', detail.name],
            ['Namespace', detail.namespace],
            ['Created', detail.createdAt],
            ['UID', detail.uid],
          ]}
        />
      </Section>

      {conditions.length > 0 && (
        <Section title="Conditions">
          <div className="flex flex-col gap-1.5">
            {conditions.map((condition) => (
              <div key={condition.type}>
                <div className="flex items-baseline gap-2">
                  <span className="text-neutral-200">{condition.type}</span>
                  <span className={conditionColor(condition)}>{condition.status}</span>
                  <span className="ml-auto text-[11px] text-neutral-400">{condition.updated}</span>
                </div>
                {condition.message !== undefined && condition.message !== '' && (
                  <p className="mt-0.5 break-words text-neutral-400">{condition.message}</p>
                )}
              </div>
            ))}
          </div>
        </Section>
      )}

      {runtimeContainers.length > 0 && (
        <Section title="Containers">
          <div className="flex flex-col gap-1">
            {runtimeContainers.map((container) => (
              <div key={container.name} className="flex items-center gap-2">
                <span className={`h-2.5 w-2.5 shrink-0 rounded-sm ${containerColor(container)}`} />
                <span className="truncate text-neutral-200">{container.name}</span>
                <span className="text-neutral-400">{container.reason}</span>
                <span className="ml-auto shrink-0 text-neutral-400">
                  {container.restarts} restarts
                </span>
              </div>
            ))}
          </div>
        </Section>
      )}

      {owners.length > 0 && (
        <Section title="Owner references">
          <Pairs pairs={owners.map((owner) => [owner.kind, owner.name])} />
        </Section>
      )}

      {labels.length > 0 && (
        <Section title="Labels">
          <Pairs pairs={labels} />
        </Section>
      )}

      {annotations.length > 0 && (
        <Section title="Annotations">
          <Pairs pairs={annotations} />
        </Section>
      )}
    </div>
  );
}

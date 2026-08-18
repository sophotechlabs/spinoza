import type { FluxController, FluxOverview, FluxSync, FluxUsage } from '../lib/types';
import { cpuFromMilli, memFromMi } from '../lib/units';
import { barColor } from '../lib/metrics';

function bannerClass(ready: boolean): string {
  if (ready) {
    return 'rounded border border-ok-line-strong bg-ok-tint/40 px-3 py-2';
  }
  return 'rounded border border-warn-line-strong bg-warn-tint/40 px-3 py-2';
}

function headline(ready: boolean): string {
  if (ready) {
    return 'All systems operational';
  }
  return 'Something needs attention';
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline gap-2">
      <span className="shrink-0 text-fg-muted">{label}</span>
      <span className="truncate text-fg-strong">{value}</span>
    </div>
  );
}

function nodeLabel(overview: FluxOverview): string {
  if (overview.nodes === 1) {
    return '1 node';
  }
  return `${String(overview.nodes)} nodes`;
}

function percentOf(used: number, total: number): number {
  if (total <= 0) {
    return 0;
  }
  return Math.round((used * 100) / total);
}

function shareLabel(used: number, request: number, limit: number): string {
  const parts: string[] = [];
  if (request > 0) {
    parts.push(`${String(percentOf(used, request))}% of request`);
  }
  if (limit > 0) {
    parts.push(`${String(percentOf(used, limit))}% of limit`);
  }
  if (parts.length === 0) {
    return 'no request or limit set';
  }
  return parts.join(' · ');
}

function Meter({ label, value, percent, share }: Readonly<MeterProps>) {
  const width = Math.min(100, Math.max(0, percent));
  return (
    <div>
      <div className="flex items-baseline justify-between gap-2">
        <span className="text-fg-muted">{label}</span>
        <span className="truncate text-fg-strong">
          {value} <span className="text-fg-muted">{share}</span>
        </span>
      </div>
      <span className="mt-1 block h-1.5 overflow-hidden rounded-sm bg-surface-active">
        <span className={`block h-full ${barColor(percent)}`} style={{ width: `${width}%` }} />
      </span>
    </div>
  );
}

interface MeterProps {
  label: string;
  value: string;
  percent: number;
  share: string;
}

function Usage({ usage }: { usage: FluxUsage }) {
  if (!usage.known) {
    return <p className="text-fg-muted">Usage needs metrics-server.</p>;
  }
  return (
    <div className="flex flex-col gap-2">
      <Meter
        label="CPU"
        value={cpuFromMilli(usage.cpuMilli)}
        percent={percentOf(usage.cpuMilli, usage.cpuRequestMilli)}
        share={shareLabel(usage.cpuMilli, usage.cpuRequestMilli, usage.cpuLimitMilli)}
      />
      <Meter
        label="Memory"
        value={memFromMi(usage.memoryMi)}
        percent={percentOf(usage.memoryMi, usage.memRequestMi)}
        share={shareLabel(usage.memoryMi, usage.memRequestMi, usage.memLimitMi)}
      />
    </div>
  );
}

function Sync({ sync }: { sync: FluxSync }) {
  if (sync.kind === '') {
    return (
      <p className="text-fg-muted">
        No {sync.name} sync was found in {sync.namespace}.
      </p>
    );
  }
  return (
    <div className="flex flex-col gap-1">
      <Fact label={sync.kind} value={`${sync.namespace}/${sync.name}`} />
      {sync.url !== '' && <Fact label={sync.source} value={sync.url} />}
      {sync.ref !== '' && <Fact label="Ref" value={sync.ref} />}
      {sync.path !== '' && <Fact label="Path" value={sync.path} />}
      <Fact label="Applied revision" value={sync.revision} />
    </div>
  );
}

function controllerClass(ready: boolean): string {
  if (ready) {
    return 'text-ok';
  }
  return 'text-error';
}

function controllerState(controller: FluxController): string {
  if (controller.ready) {
    return 'Ready';
  }
  return `${String(controller.replicas)} of ${String(controller.wanted)} running`;
}

function Card({
  title,
  note,
  children,
}: {
  title: string;
  note?: string;
  children: React.ReactNode;
}) {
  return (
    <section className="rounded border border-edge bg-surface-raised p-3">
      <h2 className="text-xs font-semibold tracking-wide text-fg-soft uppercase">{title}</h2>
      {note !== undefined && <p className="mt-0.5 text-[11px] text-fg-muted">{note}</p>}
      <div className="mt-2">{children}</div>
    </section>
  );
}

export default function FluxStatus({ overview }: { overview: FluxOverview }) {
  if (overview.error !== undefined) {
    return (
      <div className="rounded border border-error-line bg-error-tint/40 px-3 py-2 text-error-strong">
        The Flux control plane could not be read: {overview.error}
      </div>
    );
  }
  if (overview.controllers.length === 0) {
    return null;
  }

  return (
    <div className="flex flex-col gap-3">
      <div className={bannerClass(overview.ready)}>
        <div className="text-sm font-semibold text-fg-strong">{headline(overview.ready)}</div>
        <div className="text-fg-soft">{overview.summary}</div>
      </div>
      <div className="grid gap-3 lg:grid-cols-2">
        <Card title="Cluster" note={`${overview.kubernetes} · ${nodeLabel(overview)}`}>
          <div className="flex flex-col gap-1">
            {overview.operator !== undefined && (
              <Fact label="Flux Operator" value={overview.operator} />
            )}
            {overview.distribution !== undefined && (
              <Fact label="Flux" value={overview.distribution} />
            )}
            <Fact label="Namespace" value={overview.namespace} />
            <Fact label="Controllers" value={String(overview.controllers.length)} />
          </div>
        </Card>
        <Card title="Controller usage">
          <Usage usage={overview.usage} />
        </Card>
        <Card title="Cluster sync">
          <Sync sync={overview.sync} />
        </Card>
        <Card title="Components">
          <table className="w-full text-left">
            <tbody>
              {overview.controllers.map((controller) => (
                <tr key={controller.name} className="border-t border-edge first:border-t-0">
                  <td className="py-1 text-fg-strong">{controller.name}</td>
                  <td className="py-1 text-fg-muted">{controller.version}</td>
                  <td className={`py-1 text-right ${controllerClass(controller.ready)}`}>
                    {controllerState(controller)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
      </div>
    </div>
  );
}

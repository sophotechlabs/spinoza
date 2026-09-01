import { useState } from 'react';
import type { FleetApp, FleetCluster, FleetImage, FleetKind, HelmRelease } from '../lib/types';
import {
  nodesLabel,
  podsLabel,
  shortKey,
  skewLabel,
  spreadLabel,
  useFleetGitops,
  useFleetReleases,
  useFleetImages,
  useFleetInventory,
  useFleetOverview,
} from '../lib/fleet';
import { nameOf, tabOn, useClustersStore } from '../store/clusters';
import { colorVar } from '../lib/clusterColor';
import { CONTROL } from '../lib/controls';
import LoadFailure from './LoadFailure';
import LoadWarning from './LoadWarning';
import Loading from './Loading';

interface FleetProps {
  onPick: (cluster: string) => void;
}

const TABS = [
  { id: 'clusters', label: 'Clusters' },
  { id: 'inventory', label: 'What is on them' },
  { id: 'releases', label: 'Releases' },
  { id: 'delivery', label: 'Delivery' },
  { id: 'images', label: 'Images' },
] as const;

type Pane = (typeof TABS)[number]['id'];

function Swatch({ cluster }: { cluster: string }) {
  const tab = useClustersStore((state) => tabOn(state.tabs, cluster));
  if (tab === null) {
    return <span className="text-fg-faint">unknown</span>;
  }
  return (
    <span className="flex items-center gap-1.5 truncate">
      <span
        aria-hidden="true"
        style={{ backgroundColor: colorVar(tab.color) }}
        className="h-2 w-2 shrink-0 rounded-sm"
      />
      <span className="truncate">{nameOf(tab)}</span>
    </span>
  );
}

function Clusters({ onPick }: FleetProps) {
  const { data, error } = useFleetOverview();
  if (data === null) {
    if (error !== null) {
      return <LoadFailure what="The fleet" message={error} />;
    }
    return <Loading what="the fleet" />;
  }
  const warnings = data.clusters.reduce((total, cluster) => total + cluster.warnings, 0);
  return (
    <div className="min-h-0 flex-1 overflow-auto">
      {data.error !== undefined && data.error !== '' && <LoadWarning message={data.error} />}
      <table className="w-full table-fixed border-collapse text-left whitespace-nowrap">
        <thead className="sticky top-0 z-10 bg-surface-raised text-fg-muted">
          <tr className="border-b border-edge">
            <th className="w-56 px-2 py-1 font-medium">Cluster</th>
            <th className="w-28 px-2 py-1 font-medium">Version</th>
            <th className="w-24 px-2 py-1 font-medium">Nodes</th>
            <th className="w-28 px-2 py-1 font-medium">Pods</th>
            <th className="w-24 px-2 py-1 font-medium">Warnings</th>
            <th className="px-2 py-1 font-medium">Trouble</th>
          </tr>
        </thead>
        <tbody>
          {data.clusters.map((one) => (
            <Line key={one.cluster} one={one} onPick={onPick} />
          ))}
          <tr className="border-t border-edge-strong text-fg-soft">
            <td className="px-2 py-1">Everything open</td>
            <td className="px-2 py-1">{warnings}</td>
            <td className="px-2 py-1">{nodesLabel(data.nodes)}</td>
            <td className="px-2 py-1">{podsLabel(data.pods)}</td>
            <td className="px-2 py-1" />
            <td className="px-2 py-1" />
          </tr>
        </tbody>
      </table>
    </div>
  );
}

function Line({ one, onPick }: { one: FleetCluster; onPick: (cluster: string) => void }) {
  return (
    <tr className="border-t border-edge hover:bg-surface-raised">
      <td className="truncate px-2 py-1">
        <button
          type="button"
          onClick={() => {
            onPick(one.cluster);
          }}
          className="max-w-full truncate text-fg-strong hover:underline"
        >
          <Swatch cluster={one.cluster} />
        </button>
      </td>
      <td className="truncate px-2 py-1 text-fg-muted">{one.version}</td>
      <td className="px-2 py-1 text-fg-soft">{nodesLabel(one.nodes)}</td>
      <td className="px-2 py-1 text-fg-soft">{podsLabel(one.pods)}</td>
      <td className="px-2 py-1 text-fg-muted">{one.warnings > 0 ? one.warnings : ''}</td>
      <td className="truncate px-2 py-1 text-warn" title={one.reason}>
        {one.reason}
      </td>
    </tr>
  );
}

function Inventory() {
  const { data, error } = useFleetInventory();
  if (data === null) {
    if (error !== null) {
      return <LoadFailure what="The fleet inventory" message={error} />;
    }
    return <Loading what="the fleet inventory" />;
  }
  return (
    <div className="min-h-0 flex-1 overflow-auto">
      {data.error !== undefined && data.error !== '' && <LoadWarning message={data.error} />}
      <ul>
        {data.kinds.map((kind) => (
          <Kind key={kind.key} kind={kind} />
        ))}
      </ul>
      {data.kinds.length === 0 && <p className="p-3 text-fg-muted">Nothing counted yet.</p>}
    </div>
  );
}

function Kind({ kind }: { kind: FleetKind }) {
  return (
    <li className="flex items-baseline gap-3 border-b border-edge px-2 py-1">
      <span className="w-56 shrink-0 truncate text-fg-strong">{shortKey(kind.key)}</span>
      <span className="w-16 shrink-0 text-right text-fg-soft">{kind.total}</span>
      <span className="w-20 shrink-0 text-right text-error">
        {kind.failing !== undefined && kind.failing > 0 ? `${String(kind.failing)} unwell` : ''}
      </span>
      <span className="flex min-w-0 flex-1 gap-3 text-fg-muted">
        {Object.entries(kind.perCluster).map(([cluster, count]) => (
          <span key={cluster} className="flex shrink-0 items-center gap-1.5">
            <Swatch cluster={cluster} />
            {count}
          </span>
        ))}
      </span>
    </li>
  );
}

function Images() {
  const { data, error } = useFleetImages();
  if (data === null) {
    if (error !== null) {
      return <LoadFailure what="The fleet images" message={error} />;
    }
    return <Loading what="the fleet images" />;
  }
  return (
    <div className="min-h-0 flex-1 overflow-auto">
      {data.error !== undefined && data.error !== '' && <LoadWarning message={data.error} />}
      <ul>
        {data.images.map((image) => (
          <Image key={image.image} image={image} />
        ))}
      </ul>
      {data.images.length === 0 && <p className="p-3 text-fg-muted">No images found.</p>}
    </div>
  );
}

function Image({ image }: { image: FleetImage }) {
  const skew = skewLabel(image.skew);
  return (
    <li className="flex items-baseline gap-3 border-b border-edge px-2 py-1">
      <span className="min-w-0 flex-1 truncate text-fg-strong" title={image.image}>
        {image.image}
      </span>
      <span className="w-16 shrink-0 text-right text-fg-muted">{image.pods} pods</span>
      <span className="flex w-64 shrink-0 gap-2">
        {image.clusters.map((cluster) => (
          <Swatch key={cluster} cluster={cluster} />
        ))}
      </span>
      <span className="w-64 shrink-0 truncate text-warn" title={skew}>
        {skew !== '' && `${image.repo} is at ${skew}`}
      </span>
    </li>
  );
}

function Releases() {
  const { data, error } = useFleetReleases();
  if (data === null) {
    if (error !== null) {
      return <LoadFailure what="The fleet releases" message={error} />;
    }
    return <Loading what="the fleet releases" />;
  }
  return (
    <div className="min-h-0 flex-1 overflow-auto">
      {data.error !== undefined && data.error !== '' && <LoadWarning message={data.error} />}
      <ul>
        {data.releases.map((one) => (
          <Release key={`${one.cluster ?? ''}/${one.namespace}/${one.name}`} one={one} />
        ))}
      </ul>
      {data.releases.length === 0 && <p className="p-3 text-fg-muted">No releases installed.</p>}
    </div>
  );
}

function Release({ one }: { one: HelmRelease }) {
  return (
    <li className="flex items-baseline gap-3 border-b border-edge px-2 py-1">
      <span className="w-56 shrink-0 truncate text-fg-strong" title={one.name}>
        {one.name}
      </span>
      <span className="w-40 shrink-0 truncate text-fg-muted">{one.namespace}</span>
      <span className="w-32 shrink-0 truncate text-fg-soft">{one.chartVersion}</span>
      <span className="w-40 shrink-0">
        <Swatch cluster={one.cluster ?? ''} />
      </span>
      <span className="min-w-0 flex-1 truncate text-warn" title={one.skew}>
        {one.skew !== undefined && one.skew !== '' && `${one.chart} is at ${one.skew}`}
      </span>
    </li>
  );
}

function Delivery() {
  const open = useClustersStore((state) => state.tabs.length);
  const { data, error } = useFleetGitops();
  if (data === null) {
    if (error !== null) {
      return <LoadFailure what="The fleet delivery" message={error} />;
    }
    return <Loading what="the fleet delivery" />;
  }
  return (
    <div className="min-h-0 flex-1 overflow-auto">
      {data.error !== undefined && data.error !== '' && <LoadWarning message={data.error} />}
      <ul>
        {data.apps.map((app) => (
          <App key={`${app.cluster}/${app.namespace}/${app.name}`} app={app} open={open} />
        ))}
      </ul>
      {data.apps.length === 0 && <p className="p-3 text-fg-muted">Nothing is delivered here.</p>}
    </div>
  );
}

function App({ app, open }: { app: FleetApp; open: number }) {
  const spread = spreadLabel(app.spread, open);
  return (
    <li className="flex items-baseline gap-3 border-b border-edge px-2 py-1">
      <span className="w-56 shrink-0 truncate text-fg-strong" title={app.name}>
        {app.name}
      </span>
      <span className="w-20 shrink-0 truncate text-fg-muted">{app.engine}</span>
      <span className="w-40 shrink-0 truncate text-fg-muted">{app.namespace}</span>
      <span className="w-40 shrink-0">
        <Swatch cluster={app.cluster} />
      </span>
      <span className="w-24 shrink-0 truncate text-fg-soft">{app.ready}</span>
      <span className="min-w-0 flex-1 truncate text-fg-muted">{spread}</span>
    </li>
  );
}

export default function Fleet({ onPick }: FleetProps) {
  const [pane, setPane] = useState<Pane>('clusters');
  return (
    <div className="flex h-full min-h-0 flex-col text-xs">
      <div className="flex shrink-0 items-center gap-2 border-b border-edge px-2 py-1.5">
        {TABS.map((one) => (
          <button
            key={one.id}
            type="button"
            aria-current={pane === one.id}
            onClick={() => {
              setPane(one.id);
            }}
            className={`${CONTROL} border-edge-strong text-fg-soft aria-[current=true]:bg-surface-active aria-[current=true]:text-fg`}
          >
            {one.label}
          </button>
        ))}
      </div>
      {pane === 'clusters' && <Clusters onPick={onPick} />}
      {pane === 'inventory' && <Inventory />}
      {pane === 'releases' && <Releases />}
      {pane === 'delivery' && <Delivery />}
      {pane === 'images' && <Images />}
    </div>
  );
}

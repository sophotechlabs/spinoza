import { useEffect, useState } from 'react';
import type {
  ContainerState,
  FluxResource,
  GraphNode,
  ObjectRef,
  ResourceDescriptor,
  Row,
  View,
} from './lib/types';
import { useResourceFeed } from './lib/feed';
import { refFromFlux, refFromNode, refFromRow } from './lib/refs';
import Sidebar from './components/Sidebar';
import TopBar from './components/TopBar';
import ResourceTable from './components/ResourceTable';
import InspectDrawer from './components/InspectDrawer';
import GitopsGraph from './components/GitopsGraph';
import FluxDashboard from './components/FluxDashboard';
import FluxTiles from './components/FluxTiles';
import FluxResources from './components/FluxResources';
import BottomDock from './components/BottomDock';
import type { PodTarget } from './components/BottomDock';

const MAIN_SUB_ID = 'main';

function containerNames(containers: ContainerState[]): string[] {
  const regular = containers.filter((container) => !container.init);
  const init = containers.filter((container) => container.init);
  return [...regular, ...init].map((container) => container.name);
}

function podTarget(row: Row | null): PodTarget | null {
  if (row === null) {
    return null;
  }
  if (row.containers === undefined) {
    return null;
  }
  if (row.containers.length === 0) {
    return null;
  }
  return {
    namespace: row.namespace,
    name: row.name,
    containers: containerNames(row.containers),
  };
}

export default function App() {
  const feed = useResourceFeed();
  const [view, setView] = useState<View>('resources');
  const [active, setActive] = useState<ResourceDescriptor | null>(null);
  const [selected, setSelected] = useState<Row | null>(null);
  const [target, setTarget] = useState<ObjectRef | null>(null);

  const { subscribe, unsubscribe, subscribeLogs, unsubscribeLogs } = feed;

  useEffect(() => {
    if (active === null) {
      return;
    }
    subscribe(MAIN_SUB_ID, active, '');
    return () => {
      unsubscribe(MAIN_SUB_ID);
    };
  }, [active, subscribe, unsubscribe]);

  function clearSelection() {
    setSelected(null);
    setTarget(null);
  }

  function handleSelectResource(descriptor: ResourceDescriptor) {
    setActive(descriptor);
    clearSelection();
    setView('resources');
  }

  function switchView(next: View) {
    setView(next);
    clearSelection();
  }

  function handleSelectRow(row: Row) {
    setSelected(row);
    setTarget(refFromRow(active, row));
  }

  function handleSelectNode(node: GraphNode) {
    setSelected(null);
    setTarget(refFromNode(node));
  }

  function handleSelectFlux(resource: FluxResource) {
    setSelected(null);
    setTarget(refFromFlux(resource));
  }

  let mainArea = (
    <ResourceTable
      active={active}
      subId={MAIN_SUB_ID}
      selected={selected}
      onSelect={handleSelectRow}
    />
  );
  if (view === 'gitops') {
    mainArea = <GitopsGraph onSelect={handleSelectNode} />;
  }
  if (view === 'flux') {
    mainArea = <FluxDashboard onSelect={handleSelectFlux} />;
  }
  if (view === 'flux-tiles') {
    mainArea = <FluxTiles onSelect={handleSelectFlux} />;
  }
  if (view === 'flux-resources') {
    mainArea = <FluxResources onSelect={handleSelectFlux} />;
  }

  return (
    <div className="flex h-screen flex-col bg-neutral-950 font-mono text-sm text-neutral-200">
      <TopBar status={feed.status} view={view} onReconnect={feed.reconnect} />
      <div className="flex min-h-0 flex-1">
        <Sidebar
          view={view}
          activeResource={active}
          onSelect={handleSelectResource}
          onSelectGitops={() => {
            switchView('gitops');
          }}
          onSelectFlux={() => {
            switchView('flux');
          }}
          onSelectTiles={() => {
            switchView('flux-tiles');
          }}
          onSelectResources={() => {
            switchView('flux-resources');
          }}
        />
        <div className="flex min-h-0 flex-1 flex-col">
          <div className="min-h-0 flex-1">{mainArea}</div>
          <BottomDock
            pod={podTarget(selected)}
            subscribeLogs={subscribeLogs}
            unsubscribeLogs={unsubscribeLogs}
          />
        </div>
        <InspectDrawer
          target={target}
          containers={selected?.containers}
          onClose={clearSelection}
          onDeleted={clearSelection}
        />
      </div>
    </div>
  );
}

import { useEffect, useRef, useState } from 'react';
import type {
  FluxResource,
  GraphNode,
  ObjectRef,
  ResourceDescriptor,
  Row,
  View,
} from './lib/types';
import { containerNames } from './lib/containers';
import { useResourceFeed } from './lib/feed';
import { useSubRow } from './store/resources';
import { refFromFlux, refFromNode, refFromRow } from './lib/refs';
import Sidebar from './components/Sidebar';
import TopBar from './components/TopBar';
import ResourceTable from './components/ResourceTable';
import InspectDrawer from './components/InspectDrawer';
import GitopsGraph from './components/GitopsGraph';
import FluxList from './components/FluxList';
import FluxOverview from './components/FluxOverview';
import FluxRoles from './components/FluxRoles';
import BottomDock from './components/BottomDock';
import type { PodTarget } from './components/BottomDock';

const FIRST_SUB_ID = 'main#0';

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
  const [subId, setSubId] = useState(FIRST_SUB_ID);
  const [contextEpoch, setContextEpoch] = useState(0);
  const subSeq = useRef(0);
  const [selectedUid, setSelectedUid] = useState<string | null>(null);
  const [target, setTarget] = useState<ObjectRef | null>(null);

  const { subscribe, unsubscribe, subscribeLogs, unsubscribeLogs } = feed;
  const selected = useSubRow(subId, selectedUid);

  useEffect(() => {
    if (active === null) {
      return;
    }
    subscribe(subId, active, '');
    return () => {
      unsubscribe(subId);
    };
  }, [active, subId, subscribe, unsubscribe]);

  function clearSelection() {
    setSelectedUid(null);
    setTarget(null);
  }

  function handleSelectResource(descriptor: ResourceDescriptor) {
    setActive(descriptor);
    subSeq.current += 1;
    setSubId(`main#${String(subSeq.current)}`);
    clearSelection();
    setView('resources');
  }

  function handleContextChanged() {
    setActive(null);
    clearSelection();
    setContextEpoch((epoch) => epoch + 1);
    feed.reconnect();
  }

  function switchView(next: View) {
    setView(next);
    clearSelection();
  }

  function handleSelectRow(row: Row) {
    setSelectedUid(row.uid);
    setTarget(refFromRow(active, row));
  }

  function handleSelectNode(node: GraphNode) {
    setSelectedUid(null);
    setTarget(refFromNode(node));
  }

  function handleSelectFlux(resource: FluxResource) {
    setSelectedUid(null);
    setTarget(refFromFlux(resource));
  }

  let mainArea = (
    <ResourceTable active={active} subId={subId} selected={selected} onSelect={handleSelectRow} />
  );
  if (view === 'gitops') {
    mainArea = <GitopsGraph onSelect={handleSelectNode} />;
  }
  if (view === 'flux-list') {
    mainArea = <FluxList onSelect={handleSelectFlux} />;
  }
  if (view === 'flux-overview') {
    mainArea = <FluxOverview onSelect={handleSelectFlux} />;
  }
  if (view === 'flux-roles') {
    mainArea = <FluxRoles onSelect={handleSelectFlux} />;
  }

  return (
    <div className="flex h-screen flex-col bg-neutral-950 font-mono text-sm text-neutral-200">
      <TopBar
        status={feed.status}
        view={view}
        onReconnect={feed.reconnect}
        onContextChanged={handleContextChanged}
      />
      <div className="flex min-h-0 flex-1">
        <Sidebar
          epoch={contextEpoch}
          view={view}
          activeResource={active}
          onSelect={handleSelectResource}
          onSelectGraph={() => {
            switchView('gitops');
          }}
          onSelectList={() => {
            switchView('flux-list');
          }}
          onSelectOverview={() => {
            switchView('flux-overview');
          }}
          onSelectRoles={() => {
            switchView('flux-roles');
          }}
        />
        <div className="flex min-h-0 min-w-0 flex-1 flex-col">
          <div className="min-h-0 min-w-0 flex-1">{mainArea}</div>
          <BottomDock
            pod={podTarget(selected)}
            subscribeLogs={subscribeLogs}
            unsubscribeLogs={unsubscribeLogs}
          />
        </div>
        <InspectDrawer
          target={target}
          containers={selected?.containers}
          subscribeLogs={subscribeLogs}
          unsubscribeLogs={unsubscribeLogs}
          onClose={clearSelection}
          onDeleted={clearSelection}
        />
      </div>
    </div>
  );
}

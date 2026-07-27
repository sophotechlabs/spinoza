import { useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import type { GraphNode, ResourceDescriptor, Row, View } from './lib/types';
import { useResourceFeed } from './lib/feed';
import Sidebar from './components/Sidebar';
import TopBar from './components/TopBar';
import ResourceTable from './components/ResourceTable';
import DetailsDrawer from './components/DetailsDrawer';
import GitopsGraph from './components/GitopsGraph';
import GitopsNodePanel from './components/GitopsNodePanel';
import FluxDashboard from './components/FluxDashboard';
import FluxTiles from './components/FluxTiles';
import FluxResources from './components/FluxResources';
import BottomDock from './components/BottomDock';

const MAIN_SUB_ID = 'main';

export default function App() {
  const feed = useResourceFeed();
  const [view, setView] = useState<View>('resources');
  const [active, setActive] = useState<ResourceDescriptor | null>(null);
  const [selected, setSelected] = useState<Row | null>(null);
  const [selectedNode, setSelectedNode] = useState<GraphNode | null>(null);

  const { subscribe, unsubscribe } = feed;

  useEffect(() => {
    if (active === null) {
      return;
    }
    subscribe(MAIN_SUB_ID, active, '');
    return () => {
      unsubscribe(MAIN_SUB_ID);
    };
  }, [active, subscribe, unsubscribe]);

  function handleSelectResource(descriptor: ResourceDescriptor) {
    setActive(descriptor);
    setSelected(null);
    setView('resources');
  }

  function handleSelectGitops() {
    setView('gitops');
  }

  function handleSelectFlux() {
    setView('flux');
  }

  function handleSelectTiles() {
    setView('flux-tiles');
  }

  function handleSelectResources() {
    setView('flux-resources');
  }

  function handleSelectRow(row: Row) {
    setSelected(row);
  }

  function handleCloseRow() {
    setSelected(null);
  }

  function handleSelectNode(node: GraphNode) {
    setSelectedNode(node);
  }

  function handleCloseNode() {
    setSelectedNode(null);
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
    mainArea = <FluxDashboard />;
  }
  if (view === 'flux-tiles') {
    mainArea = <FluxTiles />;
  }
  if (view === 'flux-resources') {
    mainArea = <FluxResources />;
  }

  let sidePanel: ReactNode = <DetailsDrawer row={selected} onClose={handleCloseRow} />;
  if (view === 'gitops') {
    sidePanel = <GitopsNodePanel node={selectedNode} onClose={handleCloseNode} />;
  }
  if (view === 'flux') {
    sidePanel = null;
  }
  if (view === 'flux-tiles') {
    sidePanel = null;
  }
  if (view === 'flux-resources') {
    sidePanel = null;
  }

  return (
    <div className="flex h-screen flex-col bg-neutral-950 font-mono text-sm text-neutral-200">
      <TopBar status={feed.status} view={view} onReconnect={feed.reconnect} />
      <div className="flex min-h-0 flex-1">
        <Sidebar
          view={view}
          activeResource={active}
          onSelect={handleSelectResource}
          onSelectGitops={handleSelectGitops}
          onSelectFlux={handleSelectFlux}
          onSelectTiles={handleSelectTiles}
          onSelectResources={handleSelectResources}
        />
        <div className="flex min-h-0 flex-1 flex-col">
          <div className="min-h-0 flex-1">{mainArea}</div>
          <BottomDock />
        </div>
        {sidePanel}
      </div>
    </div>
  );
}

import { useEffect, useMemo, useRef, useState } from 'react';
import type { FluxResource, GraphNode, ResourceDescriptor, Row, View } from './lib/types';
import { useResourceFeed } from './lib/feed';
import { fetchContexts } from './lib/contexts';
import { descriptorOf, documentTitle, resourceKey, useRouter } from './lib/router';
import type { Selection } from './lib/refs';
import { refFromFlux, refFromNode, refFromRow, useRowForRef } from './lib/refs';
import Sidebar from './components/Sidebar';
import TopBar from './components/TopBar';
import ErrorBoundary from './components/ErrorBoundary';
import Toasts from './components/Toasts';
import ResourceTable from './components/ResourceTable';
import PanelLayout from './components/PanelLayout';
import GitopsGraph from './components/GitopsGraph';
import FluxList from './components/FluxList';
import FluxOverview from './components/FluxOverview';
import FluxRoles from './components/FluxRoles';

const FIRST_SUB_ID = 'main#0';

export default function App() {
  const feed = useResourceFeed();
  const { route, navigate, replace } = useRouter();
  const [contextEpoch, setContextEpoch] = useState(0);
  const [contextName, setContextName] = useState('');
  const subSeq = useRef(0);
  const [subId, setSubId] = useState(FIRST_SUB_ID);

  const key = resourceKey(route.resource);
  const [lastKey, setLastKey] = useState(key);
  if (key !== lastKey) {
    setLastKey(key);
    subSeq.current += 1;
    setSubId(`main#${String(subSeq.current)}`);
  }

  const active = useMemo(() => {
    if (route.resource === null) {
      return null;
    }
    return descriptorOf(route.resource);
  }, [route.resource]);

  const { subscribe, unsubscribe, subscribeLogs, unsubscribeLogs } = feed;
  const selectedRow = useRowForRef(subId, active, route.selection);
  const selection = useMemo<Selection | null>(() => {
    if (route.selection === null) {
      return null;
    }
    return { ref: route.selection, row: selectedRow };
  }, [route.selection, selectedRow]);

  useEffect(() => {
    if (active === null) {
      return;
    }
    subscribe(subId, active, '');
    return () => {
      unsubscribe(subId);
    };
  }, [active, subId, subscribe, unsubscribe]);

  useEffect(() => {
    let live = true;
    fetchContexts()
      .then((list) => {
        if (live) {
          setContextName(list.current);
        }
      })
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [contextEpoch]);

  useEffect(() => {
    if (contextName === '') {
      return;
    }
    if (route.context === contextName) {
      return;
    }
    if (route.context === '') {
      replace({ ...route, context: contextName });
      return;
    }
    replace({ ...route, context: contextName, selection: null });
  }, [contextName, route, replace]);

  useEffect(() => {
    document.title = documentTitle(route);
  }, [route]);

  function clearSelection() {
    navigate({ ...route, selection: null });
  }

  function handleSelectResource(descriptor: ResourceDescriptor) {
    navigate({
      context: route.context,
      view: 'resources',
      resource: {
        group: descriptor.group,
        version: descriptor.version,
        resource: descriptor.resource,
        kind: descriptor.kind,
      },
      selection: null,
    });
  }

  function handleContextChanged() {
    navigate({ context: '', view: route.view, resource: null, selection: null });
    setContextName('');
    setContextEpoch((epoch) => epoch + 1);
    feed.reconnect();
  }

  function handleSelectView(next: View) {
    navigate({ ...route, view: next, selection: null });
  }

  function handleSelectRow(row: Row) {
    navigate({ ...route, selection: refFromRow(active, row) });
  }

  function handleSelectNode(node: GraphNode) {
    navigate({ ...route, selection: refFromNode(node) });
  }

  function handleSelectFlux(resource: FluxResource) {
    navigate({ ...route, selection: refFromFlux(resource) });
  }

  let mainArea = (
    <ResourceTable
      active={active}
      subId={subId}
      selected={selectedRow}
      onSelect={handleSelectRow}
    />
  );
  if (route.view === 'gitops') {
    mainArea = <GitopsGraph onSelect={handleSelectNode} />;
  }
  if (route.view === 'flux-list') {
    mainArea = <FluxList onSelect={handleSelectFlux} />;
  }
  if (route.view === 'flux-overview') {
    mainArea = <FluxOverview onSelect={handleSelectFlux} />;
  }
  if (route.view === 'flux-roles') {
    mainArea = <FluxRoles onSelect={handleSelectFlux} />;
  }

  return (
    <div className="flex h-screen flex-col bg-surface font-mono text-sm text-fg">
      <TopBar
        status={feed.status}
        view={route.view}
        onReconnect={feed.reconnect}
        onContextChanged={handleContextChanged}
      />
      <div className="flex min-h-0 flex-1">
        <Sidebar
          epoch={contextEpoch}
          view={route.view}
          activeResource={active}
          onSelect={handleSelectResource}
          onSelectView={handleSelectView}
        />
        <PanelLayout
          selection={selection}
          subscribeLogs={subscribeLogs}
          unsubscribeLogs={unsubscribeLogs}
          onClose={clearSelection}
          onDeleted={clearSelection}
        >
          <ErrorBoundary label={route.view}>{mainArea}</ErrorBoundary>
        </PanelLayout>
      </div>
      <Toasts />
    </div>
  );
}

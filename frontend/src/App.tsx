import { Suspense, lazy, useEffect, useMemo, useRef, useState } from 'react';
import type {
  FluxResource,
  GraphNode,
  ObjectRef,
  ResourceDescriptor,
  Row,
  View,
} from './lib/types';
import { offline, useResourceFeed } from './lib/feed';
import { fetchContexts } from './lib/contexts';
import { descriptorOf, documentTitle, resourceKey, useRouter } from './lib/router';
import type { Selection } from './lib/refs';
import { refFromFlux, refFromNode, refFromRow, useRowForRef } from './lib/refs';
import { bumpClusterEpoch, useClusterEpoch } from './store/cluster';
import { clearForwards } from './lib/portForward';
import { focusFilter, useHotkeys } from './lib/hotkeys';
import { mayDiscard } from './lib/unsaved';
import { clearRecents, rememberObject } from './store/recents';
import { notifyOk } from './store/toasts';
import Sidebar from './components/Sidebar';
import TopBar from './components/TopBar';
import ErrorBoundary from './components/ErrorBoundary';
import Toasts from './components/Toasts';
import TooltipHost from './components/TooltipHost';
import ResourceTable from './components/ResourceTable';
import PanelLayout from './components/PanelLayout';
import ClusterOverview from './components/ClusterOverview';
import HelmReleases from './components/HelmReleases';
import FluxList from './components/FluxList';
import FluxOverview from './components/FluxOverview';
import FluxRoles from './components/FluxRoles';
import Loading from './components/Loading';
import SettingsDialog from './components/SettingsDialog';
import ConnectionBanner from './components/ConnectionBanner';
import CommandPalette from './components/CommandPalette';
import type { Section } from './components/SettingsDialog';

const GitopsGraph = lazy(() => import('./components/GitopsGraph'));

const FIRST_SUB_ID = 'main#0';
const MAIN_ID = 'content';

function staleClass(stale: boolean): string {
  if (stale) {
    return 'opacity-60';
  }
  return '';
}

export default function App() {
  const feed = useResourceFeed();
  const { route, navigate, replace } = useRouter();
  const contextEpoch = useClusterEpoch();
  const [contextName, setContextName] = useState('');
  const subSeq = useRef(0);
  const [subId, setSubId] = useState(FIRST_SUB_ID);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [settingsSection, setSettingsSection] = useState<Section>('Appearance');

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
          setContextName(list.current.name);
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

  const [wasDown, setWasDown] = useState(false);
  useEffect(() => {
    if (feed.status === 'disconnected') {
      setWasDown(true);
      return;
    }
    if (feed.status !== 'connected') {
      return;
    }
    if (!wasDown) {
      return;
    }
    setWasDown(false);
    notifyOk('Reconnected to the cluster');
  }, [feed.status, wasDown]);

  function clearSelection() {
    if (!mayDiscard()) {
      return;
    }
    navigate({ ...route, selection: null });
  }

  function openSettings(section: Section) {
    setSettingsSection(section);
    setSettingsOpen(true);
  }

  function handleEscape() {
    if (paletteOpen) {
      setPaletteOpen(false);
      return;
    }
    if (settingsOpen) {
      setSettingsOpen(false);
      return;
    }
    if (route.selection !== null) {
      clearSelection();
    }
  }

  useHotkeys({
    palette: () => {
      setPaletteOpen(true);
    },
    filter: focusFilter,
    help: () => {
      openSettings('Keyboard');
    },
    close: handleEscape,
  });

  function handleSelectResource(descriptor: ResourceDescriptor) {
    if (!mayDiscard()) {
      return;
    }
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
    clearRecents();
    clearForwards();
    bumpClusterEpoch();
    feed.reconnect();
  }

  function handleSelectView(next: View) {
    if (!mayDiscard()) {
      return;
    }
    navigate({ ...route, view: next, selection: null });
  }

  function remember(ref: ObjectRef | null) {
    if (!mayDiscard()) {
      return;
    }
    if (ref !== null) {
      rememberObject(ref);
    }
    navigate({ ...route, selection: ref });
  }

  function handleSelectRow(row: Row) {
    remember(refFromRow(active, row));
  }

  function handleSelectNode(node: GraphNode) {
    remember(refFromNode(node));
  }

  function handleSelectFlux(resource: FluxResource) {
    remember(refFromFlux(resource));
  }

  const stale = offline(feed.status, feed.attempt);

  let mainArea = (
    <ResourceTable
      active={active}
      subId={subId}
      selected={selectedRow}
      onSelect={handleSelectRow}
    />
  );
  if (route.view === 'cluster') {
    mainArea = <ClusterOverview />;
  }
  if (route.view === 'helm') {
    mainArea = <HelmReleases onSelectResource={remember} />;
  }
  if (route.view === 'gitops') {
    mainArea = (
      <Suspense fallback={<Loading what="graph" />}>
        <GitopsGraph onSelect={handleSelectNode} />
      </Suspense>
    );
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
      <a
        href={`#${MAIN_ID}`}
        className="sr-only focus:not-sr-only focus:absolute focus:z-50 focus:m-2 focus:rounded focus:border focus:border-edge-strong focus:bg-surface-raised focus:px-3 focus:py-2 focus:text-fg"
      >
        Skip to the content
      </a>
      <h1 className="sr-only">Spinoza</h1>
      <ErrorBoundary label="The top bar">
        <TopBar
          status={feed.status}
          view={route.view}
          onReconnect={feed.reconnect}
          onContextChanged={handleContextChanged}
          onOpenPalette={() => {
            setPaletteOpen(true);
          }}
          onOpenSettings={() => {
            openSettings('Appearance');
          }}
        />
      </ErrorBoundary>
      <ConnectionBanner status={feed.status} attempt={feed.attempt} onReconnect={feed.reconnect} />
      <div className="flex min-h-0 flex-1">
        <ErrorBoundary label="The sidebar">
          <Sidebar
            view={route.view}
            activeResource={active}
            onSelect={handleSelectResource}
            onSelectView={handleSelectView}
          />
        </ErrorBoundary>
        <main
          id={MAIN_ID}
          tabIndex={-1}
          aria-busy={stale}
          className={`flex min-h-0 min-w-0 flex-1 ${staleClass(stale)}`}
        >
          <PanelLayout
            selection={selection}
            subscribeLogs={subscribeLogs}
            unsubscribeLogs={unsubscribeLogs}
            onClose={clearSelection}
            onDeleted={clearSelection}
          >
            <ErrorBoundary label={route.view}>{mainArea}</ErrorBoundary>
          </PanelLayout>
        </main>
      </div>
      <Toasts />
      <TooltipHost />
      <ErrorBoundary label="The command palette">
        <CommandPalette
          open={paletteOpen}
          onClose={() => {
            setPaletteOpen(false);
          }}
          onSelectView={handleSelectView}
          onSelectResource={handleSelectResource}
          onSelectObject={remember}
        />
      </ErrorBoundary>
      <ErrorBoundary label="Settings">
        <SettingsDialog
          open={settingsOpen}
          section={settingsSection}
          onClose={() => {
            setSettingsOpen(false);
          }}
        />
      </ErrorBoundary>
    </div>
  );
}

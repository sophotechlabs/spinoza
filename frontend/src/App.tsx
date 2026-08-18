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
import { clearHistory } from './store/toasts';
import { DEFAULT_NAMESPACE, useNamespace, useNamespaceStore } from './store/namespace';
import { EVERY_NAMESPACE, ONLY_DEFAULT } from './lib/settings';
import { namespaceAnswered, useSettingsStore } from './store/settings';
import { podsIn, worthAsking } from './lib/namespaceOffer';
import { nameChips } from './lib/filterChips';
import { kindScope } from './lib/catalog';
import { clearCatalog, useCategories, useCounts } from './store/catalog';
import { clearFilters, imposeChips } from './store/filters';
import { tableKey } from './lib/tableState';
import type { PaletteOpen } from './lib/palette';
import { clearTerminals } from './store/terminals';
import { revealDetails } from './store/panels';
import { askToast, notifyOk } from './store/toasts';
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
import FluxRoles from './components/FluxRoles';
import ArgoApps from './components/ArgoApps';
import ArgoList from './components/ArgoList';
import Loading from './components/Loading';
import SettingsDialog from './components/SettingsDialog';
import ConnectionBanner from './components/ConnectionBanner';
import KubeconfigBanner from './components/KubeconfigBanner';
import ProtectionPrompt from './components/ProtectionPrompt';
import MovedToDesktop from './components/MovedToDesktop';
import CommandPalette from './components/CommandPalette';
import type { Section } from './components/SettingsDialog';

const GitopsGraph = lazy(() => import('./components/GitopsGraph'));
const ArgoGraph = lazy(() => import('./components/ArgoGraph'));

function pickerScope(view: View, scope: boolean | null): boolean | null {
  if (view !== 'resources') {
    return null;
  }
  return scope;
}

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
  const [moved, setMoved] = useState(false);
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

  const categories = useCategories();
  const scope = useMemo(() => kindScope(categories, route.resource), [categories, route.resource]);

  const { subscribe, unsubscribe, loadMore, subscribeLogs, unsubscribeLogs } = feed;
  const namespace = useNamespace();
  const chooseNamespace = useNamespaceStore((state) => state.choose);
  const openOnStart = useNamespaceStore((state) => state.openOn);
  const resetNamespace = useNamespaceStore((state) => state.reset);
  const counts = useCounts();
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
    subscribe(subId, active, namespace);
    return () => {
      unsubscribe(subId);
    };
  }, [active, namespace, subId, subscribe, unsubscribe]);

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

  useEffect(() => {
    if (contextName === '') {
      return;
    }
    openOnStart(contextName);
  }, [contextName, openOnStart]);

  useEffect(() => {
    if (!worthAsking(contextName, counts, namespaceAnswered(contextName))) {
      return;
    }
    useSettingsStore.getState().setNamespaceStart(contextName, EVERY_NAMESPACE);
    askToast(
      `Watching every namespace on ${contextName} holds all ${String(podsIn(counts))} pods in memory here.`,
      {
        label: 'Open on default',
        run: () => {
          useSettingsStore.getState().setNamespaceStart(contextName, ONLY_DEFAULT);
          chooseNamespace(DEFAULT_NAMESPACE);
        },
      },
    );
  }, [chooseNamespace, contextName, counts]);

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
    clearHistory();
    clearTerminals();
    clearForwards();
    clearFilters();
    clearCatalog();
    resetNamespace();
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
      revealDetails();
    }
    navigate({ ...route, selection: ref });
  }

  function openFound(found: PaletteOpen) {
    if (!mayDiscard()) {
      return;
    }
    rememberObject(found.ref);
    revealDetails();
    if (found.type === null) {
      navigate({ ...route, selection: found.ref });
      return;
    }
    if (found.ref.namespace !== '') {
      chooseNamespace(found.ref.namespace);
    }
    imposeChips(tableKey(found.type), nameChips(found.filter));
    navigate({
      context: route.context,
      view: 'resources',
      resource: {
        group: found.type.group,
        version: found.type.version,
        resource: found.type.resource,
        kind: found.type.kind,
      },
      selection: found.ref,
    });
  }

  function handleMore(limit: number) {
    loadMore(subId, limit);
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
      scope={scope}
      selected={selectedRow}
      onSelect={handleSelectRow}
      onMore={handleMore}
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
  if (route.view === 'flux-roles') {
    mainArea = <FluxRoles onSelect={handleSelectFlux} />;
  }
  if (route.view === 'argo-apps') {
    mainArea = <ArgoApps onSelect={remember} />;
  }
  if (route.view === 'argo-graph') {
    mainArea = (
      <Suspense fallback={<Loading what="graph" />}>
        <ArgoGraph onSelect={handleSelectNode} />
      </Suspense>
    );
  }
  if (route.view === 'argo-list') {
    mainArea = <ArgoList onSelect={remember} />;
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
          scoped={pickerScope(route.view, scope)}
          onReconnect={feed.reconnect}
          onContextChanged={handleContextChanged}
          onOpenPalette={() => {
            setPaletteOpen(true);
          }}
          onOpenSettings={() => {
            openSettings('Appearance');
          }}
          onSelectObject={remember}
          onLeftForDesktop={() => {
            setMoved(true);
          }}
        />
      </ErrorBoundary>
      <ConnectionBanner status={feed.status} attempt={feed.attempt} onReconnect={feed.reconnect} />
      <KubeconfigBanner />
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
      <ProtectionPrompt />
      <MovedToDesktop
        open={moved}
        onStay={() => {
          setMoved(false);
        }}
      />
      <ErrorBoundary label="The command palette">
        <CommandPalette
          open={paletteOpen}
          onClose={() => {
            setPaletteOpen(false);
          }}
          onSelectView={handleSelectView}
          onSelectResource={handleSelectResource}
          onOpenObject={openFound}
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

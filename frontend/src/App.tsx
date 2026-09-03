import { Suspense, lazy, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type {
  FluxResource,
  GraphNode,
  HelmRelease,
  ObjectRef,
  ReleaseRef,
  ResourceDescriptor,
  Row,
  View,
} from './lib/types';
import { offline, useResourceFeed } from './lib/feed';
import { fetchContexts } from './lib/contexts';
import { activateCluster, fetchClusters, openCluster, stillToOpen } from './lib/clusters';
import {
  activeOf,
  rememberRoute,
  useActiveCluster,
  useClustersStore,
  useTabs,
} from './store/clusters';
import { contextOf, displayName } from './lib/tabs';
import ClusterStrip from './components/ClusterStrip';
import { announceUpdate } from './lib/update';
import { watchSettings } from './lib/settingsSync';
import { useContextsStore } from './store/contexts';
import { useClusterMode } from './store/identity';
import { descriptorOf, documentTitle, resourceKey, useRouter } from './lib/router';
import type { Route } from './lib/router';
import type { Selection } from './lib/refs';
import { refFromFlux, refFromNode, refFromRow, useRowForRef } from './lib/refs';
import { bumpClusterEpoch, useClusterEpoch } from './store/cluster';
import { focusFilter, useHotkeys } from './lib/hotkeys';
import { mayDiscard } from './lib/unsaved';
import { rememberObject } from './store/recents';
import { DEFAULT_NAMESPACE, useNamespace, useNamespaceStore } from './store/namespace';
import { EVERY_NAMESPACE, ONLY_DEFAULT } from './lib/settings';
import { namespaceAnswered, useSettingsStore } from './store/settings';
import { podsIn, worthAsking } from './lib/namespaceOffer';
import { kindScope } from './lib/catalog';
import { useCatalogKnown, useCategories, useCounts } from './store/catalog';
import { useSubLimit } from './store/resources';
import type { Chip } from './lib/filterChips';
import { chipsKey, nameChips } from './lib/filterChips';
import { chipsOf, imposeChips, useChips } from './store/filters';
import { tableKey } from './lib/tableState';
import type { PaletteOpen } from './lib/palette';
import { revealDetails, revealPanel } from './store/panels';
import { askToast, notifyError, notifyOk } from './store/toasts';
import { gitopsAbsence } from './lib/gitops';
import Sidebar from './components/Sidebar';
import TopBar from './components/TopBar';
import ErrorBoundary from './components/ErrorBoundary';
import Toasts from './components/Toasts';
import TooltipHost from './components/TooltipHost';
import ResourceTable from './components/ResourceTable';
import PanelLayout from './components/PanelLayout';
import ClusterOverview from './components/ClusterOverview';
import HelmReleases from './components/HelmReleases';
import IssueQueue from './components/IssueQueue';
import FluxList from './components/FluxList';
import FluxRoles from './components/FluxRoles';
import ArgoApps from './components/ArgoApps';
import ArgoList from './components/ArgoList';
import Checks from './components/Checks';
import History from './components/History';
import Fleet from './components/Fleet';
import Rbac from './components/Rbac';
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
const TopologyGraph = lazy(() => import('./components/TopologyGraph'));
const Traffic = lazy(() => import('./components/Traffic'));

function releaseIdentity(release: ReleaseRef | null): string {
  if (release === null) {
    return '';
  }
  return `${release.namespace}/${release.name}`;
}

function pickerScope(view: View, scope: boolean | null): boolean | null {
  if (view === 'topology') {
    return true;
  }
  if (view !== 'resources') {
    return null;
  }
  return scope;
}

const FIRST_SUB_ID = 'main#0';
const MAIN_ID = 'content';

function blankRoute(context: string): Route {
  return { context, view: 'cluster', resource: null, selection: null, release: null };
}

function switchFailed(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'switching context failed';
}

async function adoptContext(name: string): Promise<string> {
  const list = await openCluster('', name);
  return contextOf(useClustersStore.getState().tabs, activeOf(list.clusters));
}

async function openRememberedTabs(): Promise<void> {
  try {
    const list = await fetchClusters();
    for (const one of stillToOpen(list)) {
      await openCluster(one.kubeconfig ?? '', one.context);
    }
  } catch {
    return;
  }
}

function windowKey(windowed: boolean, chips: Chip[]): string {
  if (!windowed) {
    return '';
  }
  return chipsKey(chips);
}

function chipsNow(active: ResourceDescriptor): Chip[] {
  return chipsOf(tableKey(active));
}

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
  const tabs = useTabs();
  const onCluster = useActiveCluster();
  const contextName = contextOf(tabs, onCluster);
  const shownAs = displayName(tabs, onCluster, contextName);
  const subSeq = useRef(0);
  const routeRef = useRef(route);
  routeRef.current = route;
  const linked = useRef(route.context);
  const adopted = useRef(false);
  const adopting = useRef(false);
  const [subId, setSubId] = useState(FIRST_SUB_ID);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [moved, setMoved] = useState(false);
  const served = useClusterMode();
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [settingsSection, setSettingsSection] = useState<Section>('Appearance');

  const active = useMemo(() => {
    if (route.resource === null) {
      return null;
    }
    return descriptorOf(route.resource);
  }, [route.resource]);

  const chips = useChips(tableKey(active));
  const windowedKinds = useRef(new Set<string>());
  const resource = resourceKey(route.resource);
  if (useSubLimit(subId) > 0) {
    windowedKinds.current.add(resource);
  }
  const key = `${resource}|${windowKey(windowedKinds.current.has(resource), chips)}`;
  const [lastKey, setLastKey] = useState(key);
  if (key !== lastKey) {
    setLastKey(key);
    subSeq.current += 1;
    setSubId(`main#${String(subSeq.current)}`);
  }

  const categories = useCategories();
  const catalogKnown = useCatalogKnown();
  const scope = useMemo(() => kindScope(categories, route.resource), [categories, route.resource]);

  const { subscribe, unsubscribe, loadMore, subscribeLogs, unsubscribeLogs } = feed;
  const namespace = useNamespace();
  const chooseNamespace = useNamespaceStore((state) => state.choose);
  const openOnStart = useNamespaceStore((state) => state.openOn);
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
    subscribe(subId, active, namespace, chipsNow(active));
    return () => {
      unsubscribe(subId);
    };
  }, [active, namespace, onCluster, subId, subscribe, unsubscribe]);

  const showActiveTab = useCallback(() => {
    const state = useClustersStore.getState();
    bumpClusterEpoch();
    const context = contextOf(state.tabs, state.active);
    const held = state.routes[state.active];
    if (held !== undefined) {
      replace({ ...held, context });
      return;
    }
    replace(blankRoute(context));
  }, [replace]);

  useEffect(() => {
    if (served) {
      showActiveTab();
      return;
    }
    void openRememberedTabs().then(() => {
      if (linked.current !== '') {
        return;
      }
      showActiveTab();
    });
  }, [served, showActiveTab]);

  useEffect(() => {
    let live = true;
    fetchContexts()
      .then((list) => {
        if (live) {
          useContextsStore.getState().setList(list);
        }
      })
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [contextEpoch]);

  useEffect(() => {
    if (served) {
      return;
    }
    void announceUpdate();
  }, [served]);

  useEffect(watchSettings, []);

  useEffect(() => {
    if (contextName === '') {
      return;
    }
    if (route.context === contextName) {
      adopted.current = true;
      return;
    }
    if (route.context === '') {
      replace({ ...route, context: contextName });
      return;
    }
    if (adopting.current) {
      return;
    }
    if (adopted.current) {
      return;
    }
    adopted.current = true;
    adopting.current = true;
    adoptContext(route.context)
      .then((name) => {
        if (name === route.context) {
          bumpClusterEpoch();
          return;
        }
        replace({ ...route, context: name, selection: null, release: null });
      })
      .catch((err: unknown) => {
        notifyError(`Opening ${route.context}: ${switchFailed(err)}`);
        replace({ ...route, context: contextName, selection: null, release: null });
      })
      .finally(() => {
        adopting.current = false;
      });
  }, [contextName, route, replace]);

  useEffect(() => {
    document.title = documentTitle(route, shownAs);
  }, [route, shownAs]);

  useEffect(() => {
    if (onCluster === '') {
      return;
    }
    if (route.context !== contextName) {
      return;
    }
    rememberRoute(onCluster, route);
  }, [contextName, onCluster, route]);

  useEffect(() => {
    if (contextName === '') {
      return;
    }
    openOnStart(contextName);
  }, [contextName, openOnStart]);

  const releaseKey = releaseIdentity(route.release);
  useEffect(() => {
    if (releaseKey === '') {
      return;
    }
    revealPanel('release');
  }, [releaseKey]);

  useEffect(() => {
    if (!worthAsking(onCluster, counts, namespaceAnswered(onCluster, contextName))) {
      return;
    }
    useSettingsStore.getState().setNamespaceStart(onCluster, EVERY_NAMESPACE);
    askToast(
      `Watching every namespace on ${shownAs} holds all ${String(podsIn(counts))} pods in memory here.`,
      {
        label: 'Open on default',
        run: () => {
          useSettingsStore.getState().setNamespaceStart(onCluster, ONLY_DEFAULT);
          chooseNamespace(DEFAULT_NAMESPACE);
        },
      },
    );
  }, [chooseNamespace, contextName, counts, onCluster, shownAs]);

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
      release: route.release,
    });
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

  function openRelease(release: HelmRelease) {
    navigate({ ...route, release: { namespace: release.namespace, name: release.name } });
  }

  function closeRelease() {
    navigate({ ...route, release: null });
  }

  function openInTable(ref: ObjectRef, kind: string) {
    if (!mayDiscard()) {
      return;
    }
    rememberObject(ref);
    if (ref.namespace !== '') {
      chooseNamespace(ref.namespace);
    }
    navigate({
      context: route.context,
      view: 'resources',
      resource: { group: ref.group, version: ref.version, resource: ref.resource, kind },
      selection: ref,
      release: route.release,
    });
  }

  function goToCluster(cluster: string) {
    if (cluster === onCluster) {
      return;
    }
    if (!mayDiscard()) {
      return;
    }
    void switchTo(cluster, null);
  }

  function openFound(found: PaletteOpen) {
    if (!mayDiscard()) {
      return;
    }
    if (found.cluster !== undefined && found.cluster !== '' && found.cluster !== onCluster) {
      void switchTo(found.cluster, found.ref);
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
      release: route.release,
    });
  }

  function handleMore(limit: number) {
    loadMore(subId, limit);
  }

  function showOn(cluster: string, ref: ObjectRef) {
    if (cluster === onCluster) {
      remember(ref);
      return;
    }
    if (!mayDiscard()) {
      return;
    }
    void switchTo(cluster, ref);
  }

  async function switchTo(cluster: string, ref: ObjectRef | null) {
    try {
      await activateCluster(cluster);
    } catch (err: unknown) {
      const state = useClustersStore.getState();
      const context = contextOf(state.tabs, cluster);
      notifyError(`Switching to ${context}: ${switchFailed(err)}`);
      return;
    }
    const state = useClustersStore.getState();
    bumpClusterEpoch();
    const context = contextOf(state.tabs, cluster);
    if (ref === null) {
      replace(state.routes[cluster] ?? blankRoute(context));
      return;
    }
    rememberObject(ref);
    revealDetails();
    replace({
      context,
      view: 'issues',
      resource: null,
      selection: ref,
      release: null,
    });
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
      cluster={shownAs}
      selected={selectedRow}
      onSelect={handleSelectRow}
      onMore={handleMore}
    />
  );
  if (route.view === 'cluster') {
    mainArea = <ClusterOverview />;
  }
  if (route.view === 'issues') {
    mainArea = <IssueQueue onSelect={remember} onSelectOn={showOn} />;
  }
  if (route.view === 'topology') {
    mainArea = (
      <Suspense fallback={<Loading what="graph" />}>
        <TopologyGraph openedOn={route.selection} onSelect={handleSelectNode} />
      </Suspense>
    );
  }
  if (route.view === 'helm') {
    mainArea = <HelmReleases selected={route.release} onSelect={openRelease} />;
  }
  if (route.view === 'checks') {
    mainArea = <Checks onOpen={openInTable} />;
  }
  if (route.view === 'history') {
    mainArea = <History onOpen={remember} />;
  }
  if (route.view === 'fleet') {
    mainArea = <Fleet onPick={goToCluster} />;
  }
  if (route.view === 'rbac') {
    mainArea = <Rbac />;
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
  if (route.view === 'traffic') {
    mainArea = (
      <Suspense fallback={<Loading what="the traffic graph" />}>
        <Traffic />
      </Suspense>
    );
  }
  if (catalogKnown) {
    const absent = gitopsAbsence(route.view, categories);
    if (absent !== null) {
      mainArea = (
        <div className="flex h-full items-center justify-center text-xs text-fg-muted">
          {absent}
        </div>
      );
    }
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
          onContextChanged={showActiveTab}
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
      {!served && (
        <ErrorBoundary label="The cluster strip">
          <ClusterStrip onShown={showActiveTab} />
        </ErrorBoundary>
      )}
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
            key={onCluster}
            selection={selection}
            release={route.release}
            kind={active}
            namespace={namespace}
            subscribeLogs={subscribeLogs}
            unsubscribeLogs={unsubscribeLogs}
            onClose={clearSelection}
            onDeleted={clearSelection}
            onSelectResource={remember}
            onOpenResource={openInTable}
            onReleaseClose={closeRelease}
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

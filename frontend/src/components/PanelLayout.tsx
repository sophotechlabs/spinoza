import { Suspense, lazy, useCallback, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import type {
  ResourceDescriptor,
  ContainerState,
  LogRequest,
  ObjectDetail,
  ObjectRef,
  ReleaseRef,
  Row,
} from '../lib/types';
import type { DockSide, PanelContext, PanelId } from '../lib/panels';
import type { Selection } from '../lib/refs';
import { DOCK_SIDES, ownsPods, panelBodyId, panelById, panelsOn, tabId } from '../lib/panels';
import { podFor } from '../lib/pods';
import { usePanelsStore } from '../store/panels';
import { containerNames } from '../lib/containers';
import { forwardKind } from '../lib/portForward';
import { isFluxObject } from '../lib/fluxActions';
import { isArgoApplication } from '../lib/argoActions';
import { hasActions } from '../lib/objectActions';
import { useObjectDetail } from '../lib/useObjectDetail';
import PanelHost from './PanelHost';
import type { PanelTab } from './PanelHost';
import PanelMount from './PanelMount';
import PanelChrome from './PanelChrome';
import InspectActions from './InspectActions';
import ArgoActions from './ArgoActions';
import InspectObjectActions from './InspectObjectActions';
import InspectPorts from './InspectPorts';
import InspectOverview from './InspectOverview';
import InspectYaml from './InspectYaml';
import InspectEvents from './InspectEvents';
import InspectLogs from './InspectLogs';
import ForwardsPanel from './ForwardsPanel';
import TerminalTab from './TerminalTab';
import ReleasePanel from './ReleasePanel';
import ComparePanel from './ComparePanel';
import Loading from './Loading';

const InspectMetrics = lazy(() => import('./InspectMetrics'));

interface PanelLayoutProps {
  selection: Selection | null;
  release: ReleaseRef | null;
  kind: ResourceDescriptor | null;
  namespace: string;
  subscribeLogs: (subId: string, request: LogRequest) => void;
  unsubscribeLogs: (subId: string) => void;
  onClose: () => void;
  onDeleted: () => void;
  onSelectResource: (ref: ObjectRef) => void;
  onOpenResource: (ref: ObjectRef, kind: string) => void;
  onReleaseClose: () => void;
  children: ReactNode;
}

type Hosts = Record<DockSide, HTMLDivElement | null>;

const NO_HOSTS: Hosts = { left: null, right: null, bottom: null };

interface RenderContext extends PanelContext {
  error: string | null;
  gone: boolean;
  open: boolean;
  subscribeLogs: (subId: string, request: LogRequest) => void;
  unsubscribeLogs: (subId: string) => void;
  reload: () => void;
  onClose: () => void;
  onDeleted: () => void;
  onSelectResource: (ref: ObjectRef) => void;
  onOpenResource: (ref: ObjectRef, kind: string) => void;
  onReleaseClose: () => void;
}

function liveContainers(selection: Selection): ContainerState[] | undefined {
  if (selection.row === null) {
    return undefined;
  }
  return selection.row.containers;
}

// A workload is tailed through its own ref, so the server can read the selector
// it puts on its pods.
function workloadOf(
  selection: Selection,
  detail: ObjectDetail,
): { group: string; version: string; resource: string } | undefined {
  if (!ownsPods(detail)) {
    return undefined;
  }
  return {
    group: selection.ref.group,
    version: selection.ref.version,
    resource: selection.ref.resource,
  };
}

function refOf(selection: Selection | null): ObjectRef | null {
  if (selection === null) {
    return null;
  }
  return selection.ref;
}

function rowOf(selection: Selection | null): Row | null {
  if (selection === null) {
    return null;
  }
  return selection.row;
}

function logContainers(states: ContainerState[] | undefined, detail: ObjectDetail): string[] {
  if (states !== undefined && states.length > 0) {
    return containerNames(states);
  }
  return detail.pod?.containers ?? [];
}

function forwardable(detail: ObjectDetail): string | null {
  if (detail.ports === undefined) {
    return null;
  }
  if (detail.ports.length === 0) {
    return null;
  }
  return forwardKind(detail.apiVersion, detail.kind);
}

type ObjectBody = (selection: Selection, detail: ObjectDetail) => ReactNode;

function objectPanel(ctx: RenderContext, body: ObjectBody): ReactNode {
  return (
    <PanelChrome target={refOf(ctx.selection)} onClose={ctx.onClose}>
      {objectBody(ctx, body)}
    </PanelChrome>
  );
}

function objectBody(ctx: RenderContext, body: ObjectBody): ReactNode {
  if (ctx.gone) {
    return (
      <div className="p-4 text-xs text-fg-muted">This object is no longer in the cluster.</div>
    );
  }
  if (ctx.error !== null) {
    return <div className="p-4 text-xs break-words text-error">{ctx.error}</div>;
  }
  if (ctx.selection === null || ctx.detail === null) {
    return <div className="p-4 text-xs text-fg-muted">Loading</div>;
  }
  return body(ctx.selection, ctx.detail);
}

const RENDERERS: Record<PanelId, (ctx: RenderContext) => ReactNode> = {
  forwards: (ctx) => (
    <div className="min-h-0 flex-1 overflow-auto text-[11px]">
      <ForwardsPanel active={ctx.open} />
    </div>
  ),
  terminal: (ctx) => <TerminalTab pod={ctx.pod} />,
  release: (ctx) => (
    <ReleasePanel
      target={ctx.release}
      onSelectResource={ctx.onSelectResource}
      onOpenResource={ctx.onOpenResource}
      onClose={ctx.onReleaseClose}
    />
  ),
  compare: (ctx) => (
    <div className="flex min-h-0 flex-1 flex-col">
      <ComparePanel
        target={refOf(ctx.selection)}
        kind={ctx.kind}
        namespace={ctx.namespace}
        onOpen={ctx.onSelectResource}
      />
    </div>
  ),
  overview: (ctx) =>
    objectPanel(ctx, (selection, detail) => (
      <div className="min-h-0 flex-1 overflow-y-auto">
        {isFluxObject(detail.apiVersion) && (
          <InspectActions
            target={selection.ref}
            suspended={detail.flux?.suspended}
            onDone={ctx.reload}
          />
        )}
        {isArgoApplication(detail.apiVersion, detail.kind) && (
          <ArgoActions target={selection.ref} onDone={ctx.reload} />
        )}
        {hasActions(selection.ref) && (
          <InspectObjectActions target={selection.ref} detail={detail} onDone={ctx.reload} />
        )}
        {forwardable(detail) !== null && detail.ports !== undefined && (
          <InspectPorts
            target={selection.ref}
            kind={forwardable(detail) ?? ''}
            ports={detail.ports}
          />
        )}
        <InspectOverview detail={detail} containers={liveContainers(selection)} />
      </div>
    )),
  yaml: (ctx) =>
    objectPanel(ctx, (selection, detail) => (
      <InspectYaml
        target={selection.ref}
        detail={detail}
        onApplied={ctx.reload}
        onDeleted={ctx.onDeleted}
      />
    )),
  events: (ctx) =>
    objectPanel(ctx, (_selection, detail) => (
      <InspectEvents namespace={detail.namespace} uid={detail.uid} active={ctx.open} />
    )),
  logs: (ctx) =>
    objectPanel(ctx, (selection, detail) => (
      <InspectLogs
        namespace={detail.namespace}
        pod={detail.name}
        containers={logContainers(liveContainers(selection), detail)}
        workload={workloadOf(selection, detail)}
        active={ctx.open}
        subscribeLogs={ctx.subscribeLogs}
        unsubscribeLogs={ctx.unsubscribeLogs}
      />
    )),
  metrics: (ctx) =>
    objectPanel(ctx, (_selection, detail) => (
      <Suspense fallback={<Loading what="charts" />}>
        <InspectMetrics namespace={detail.namespace} pod={detail.name} />
      </Suspense>
    )),
};

export default function PanelLayout({
  selection,
  release,
  kind,
  namespace,
  subscribeLogs,
  unsubscribeLogs,
  onClose,
  onDeleted,
  onSelectResource,
  onOpenResource,
  onReleaseClose,
  children,
}: PanelLayoutProps) {
  const placement = usePanelsStore((state) => state.placement);
  const move = usePanelsStore((state) => state.move);
  const chosen = usePanelsStore((state) => state.active);
  const activate = usePanelsStore((state) => state.activate);
  const [hosts, setHosts] = useState<Hosts>(NO_HOSTS);
  const [opened, setOpened] = useState<PanelId[]>([]);
  const { detail, error, gone, reload } = useObjectDetail(refOf(selection), rowOf(selection));

  const ctx: PanelContext = {
    selection,
    detail,
    pod: podFor(selection, detail),
    release,
    kind,
    namespace,
  };

  const live = DOCK_SIDES.map((side) => activeOn(side)).filter((id) => id !== null);
  const liveKey = live.join(',');

  const remember = useCallback((ids: PanelId[]) => {
    setOpened((current) => {
      const missing = ids.filter((id) => !current.includes(id));
      if (missing.length === 0) {
        return current;
      }
      return [...current, ...missing];
    });
  }, []);

  useEffect(() => {
    remember(liveKey.split(',').filter(Boolean) as PanelId[]);
  }, [liveKey, remember]);

  const registerHost = useMemo(() => {
    function setter(side: DockSide) {
      return (element: HTMLDivElement | null) => {
        setHosts((current) => ({ ...current, [side]: element }));
      };
    }
    return { left: setter('left'), right: setter('right'), bottom: setter('bottom') };
  }, []);

  function enabledOf(id: PanelId): boolean {
    return panelById(id).enabled(ctx);
  }

  function titleOf(id: PanelId): string {
    const panel = panelById(id);
    if (enabledOf(id)) {
      return `${panel.label}, drag to move`;
    }
    return panel.hint;
  }

  function tabsFor(side: DockSide): PanelTab[] {
    return panelsOn(placement, side).map((id) => ({
      id,
      label: panelById(id).label,
      disabled: !enabledOf(id),
      title: titleOf(id),
    }));
  }

  function activeOn(side: DockSide): PanelId | null {
    const here = panelsOn(placement, side);
    const wanted = chosen[side];
    if (wanted !== null && here.includes(wanted) && enabledOf(wanted)) {
      return wanted;
    }
    const usable = here.filter((id) => enabledOf(id));
    if (usable.length === 0) {
      return null;
    }
    return usable[0];
  }

  function hintFor(side: DockSide): string {
    const blocked = panelsOn(placement, side).find((id) => !enabledOf(id));
    if (blocked === undefined) {
      return 'Nothing docked here yet.';
    }
    return `${panelById(blocked).hint}.`;
  }

  function activateOn(side: DockSide) {
    return (id: PanelId) => {
      activate(side, id);
    };
  }

  function handleMove(id: PanelId, side: DockSide) {
    move(id, side);
    activate(side, id);
  }

  const mounts = DOCK_SIDES.flatMap((side) =>
    panelsOn(placement, side)
      .filter((id) => enabledOf(id))
      .filter((id) => opened.includes(id))
      .map((id) => {
        const open = activeOn(side) === id;
        const render: RenderContext = {
          ...ctx,
          error,
          gone,
          open,
          subscribeLogs,
          unsubscribeLogs,
          reload,
          onClose,
          onDeleted,
          onSelectResource,
          onOpenResource,
          onReleaseClose,
        };
        return (
          <PanelMount
            key={id}
            host={hosts[side]}
            active={open}
            label={panelById(id).label}
            id={panelBodyId(id)}
            labelledBy={tabId(id)}
          >
            {RENDERERS[id](render)}
          </PanelMount>
        );
      }),
  );

  return (
    <div className="flex min-h-0 min-w-0 flex-1 overflow-hidden">
      <PanelHost
        side="left"
        tabs={tabsFor('left')}
        active={activeOn('left')}
        onActivate={activateOn('left')}
        onMove={handleMove}
        hostRef={registerHost.left}
        emptyHint={hintFor('left')}
      />
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">
        <div className="min-h-0 min-w-0 flex-1">{children}</div>
        <PanelHost
          side="bottom"
          tabs={tabsFor('bottom')}
          active={activeOn('bottom')}
          onActivate={activateOn('bottom')}
          onMove={handleMove}
          hostRef={registerHost.bottom}
          emptyHint={hintFor('bottom')}
        />
      </div>
      <PanelHost
        side="right"
        tabs={tabsFor('right')}
        active={activeOn('right')}
        onActivate={activateOn('right')}
        onMove={handleMove}
        hostRef={registerHost.right}
        emptyHint={hintFor('right')}
      />
      {mounts}
    </div>
  );
}

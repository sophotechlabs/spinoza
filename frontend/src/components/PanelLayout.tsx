import { useCallback, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import type { ContainerState, LogRequest, ObjectDetail, ObjectRef } from '../lib/types';
import type { DockSide, PanelId } from '../lib/panels';
import type { PodTarget } from '../lib/pods';
import { DOCK_SIDES, PANEL_LABELS, panelsOn, usePlacement } from '../lib/panels';
import { containerNames } from '../lib/containers';
import { forwardKind } from '../lib/portForward';
import { isFluxObject } from '../lib/fluxActions';
import { hasActions } from '../lib/objectActions';
import { useObjectDetail } from '../lib/useObjectDetail';
import PanelHost from './PanelHost';
import type { PanelTab } from './PanelHost';
import PanelMount from './PanelMount';
import PanelChrome from './PanelChrome';
import InspectActions from './InspectActions';
import InspectObjectActions from './InspectObjectActions';
import InspectPorts from './InspectPorts';
import InspectOverview from './InspectOverview';
import InspectYaml from './InspectYaml';
import InspectEvents from './InspectEvents';
import InspectMetrics from './InspectMetrics';
import InspectLogs from './InspectLogs';
import ForwardsPanel from './ForwardsPanel';
import TerminalTab from './TerminalTab';

interface PanelLayoutProps {
  target: ObjectRef | null;
  containers?: ContainerState[];
  pod: PodTarget | null;
  subscribeLogs: (subId: string, request: LogRequest) => void;
  unsubscribeLogs: (subId: string) => void;
  onClose: () => void;
  onDeleted: () => void;
  children: ReactNode;
}

type Hosts = Record<DockSide, HTMLDivElement | null>;

const NO_HOSTS: Hosts = { left: null, right: null, bottom: null };

function logContainers(states: ContainerState[] | undefined, detail: ObjectDetail): string[] {
  if (states !== undefined && states.length > 0) {
    return containerNames(states);
  }
  return detail.containers ?? [];
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

function podOnly(detail: ObjectDetail | null): boolean {
  if (detail === null) {
    return false;
  }
  return detail.kind === 'Pod';
}

export default function PanelLayout({
  target,
  containers,
  pod,
  subscribeLogs,
  unsubscribeLogs,
  onClose,
  onDeleted,
  children,
}: PanelLayoutProps) {
  const { placement, move } = usePlacement();
  const [hosts, setHosts] = useState<Hosts>(NO_HOSTS);
  const [activeBySide, setActiveBySide] = useState<Record<DockSide, PanelId | null>>({
    left: null,
    right: null,
    bottom: null,
  });
  const [opened, setOpened] = useState<PanelId[]>([]);
  const { detail, error, reload } = useObjectDetail(target);

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
    if (id === 'forwards') {
      return true;
    }
    if (id === 'terminal') {
      return pod !== null;
    }
    if (id === 'logs' || id === 'metrics') {
      return podOnly(detail);
    }
    return target !== null;
  }

  function titleOf(id: PanelId): string {
    if (enabledOf(id)) {
      return `${PANEL_LABELS[id]} — drag to another dock to move it`;
    }
    if (id === 'terminal') {
      return 'Select a pod to open a shell in it';
    }
    if (id === 'logs' || id === 'metrics') {
      return 'Select a pod to see this';
    }
    return 'Select a row to inspect it';
  }

  function tabsFor(side: DockSide): PanelTab[] {
    return panelsOn(placement, side).map((id) => ({
      id,
      disabled: !enabledOf(id),
      title: titleOf(id),
    }));
  }

  function activeOn(side: DockSide): PanelId | null {
    const here = panelsOn(placement, side);
    const chosen = activeBySide[side];
    if (chosen !== null && here.includes(chosen) && enabledOf(chosen)) {
      return chosen;
    }
    const usable = here.filter((id) => enabledOf(id));
    if (usable.length === 0) {
      return null;
    }
    return usable[0];
  }

  function hintFor(side: DockSide): string {
    const here = panelsOn(placement, side);
    if (here.includes('terminal') && pod === null) {
      return 'Select a pod to open a shell in it.';
    }
    if (here.some((id) => id !== 'forwards' && id !== 'terminal')) {
      return 'Select a row to inspect it.';
    }
    return 'Nothing docked here yet.';
  }

  function activate(side: DockSide) {
    return (id: PanelId) => {
      setActiveBySide((current) => ({ ...current, [side]: id }));
    };
  }

  function handleMove(id: PanelId, side: DockSide) {
    move(id, side);
    setActiveBySide((current) => ({ ...current, [side]: id }));
  }

  function objectBody(node: ReactNode): ReactNode {
    if (error !== null) {
      return <div className="p-4 text-xs break-words text-red-400">{error}</div>;
    }
    if (detail === null) {
      return <div className="p-4 text-xs text-neutral-400">Loading…</div>;
    }
    return node;
  }

  function objectPanel(ref: ObjectRef | null, node: (target: ObjectRef) => ReactNode): ReactNode {
    return (
      <PanelChrome target={ref} onClose={onClose}>
        {objectBody(ref === null || detail === null ? null : node(ref))}
      </PanelChrome>
    );
  }

  function bodyOf(id: PanelId, active: boolean): ReactNode {
    if (id === 'forwards') {
      return (
        <div className="min-h-0 flex-1 overflow-auto text-[11px]">
          <ForwardsPanel active={active} />
        </div>
      );
    }
    if (id === 'terminal') {
      return <TerminalTab pod={pod} />;
    }
    if (id === 'overview') {
      return objectPanel(target, (ref) => (
        <div className="min-h-0 flex-1 overflow-y-auto">
          {detail !== null && isFluxObject(detail.apiVersion) && (
            <InspectActions target={ref} suspended={detail.suspended} onDone={reload} />
          )}
          {detail !== null && hasActions(ref) && (
            <InspectObjectActions target={ref} detail={detail} onDone={reload} />
          )}
          {detail !== null && forwardable(detail) !== null && detail.ports !== undefined && (
            <InspectPorts target={ref} kind={forwardable(detail) ?? ''} ports={detail.ports} />
          )}
          {detail !== null && <InspectOverview detail={detail} containers={containers} />}
        </div>
      ));
    }
    if (id === 'yaml') {
      return objectPanel(target, (ref) =>
        detail === null ? null : (
          <InspectYaml target={ref} detail={detail} onApplied={reload} onDeleted={onDeleted} />
        ),
      );
    }
    if (id === 'events') {
      return objectPanel(target, () =>
        detail === null ? null : <InspectEvents namespace={detail.namespace} uid={detail.uid} />,
      );
    }
    if (id === 'logs') {
      return objectPanel(target, () =>
        detail === null ? null : (
          <InspectLogs
            namespace={detail.namespace}
            pod={detail.name}
            containers={logContainers(containers, detail)}
            subscribeLogs={subscribeLogs}
            unsubscribeLogs={unsubscribeLogs}
          />
        ),
      );
    }
    return objectPanel(target, () =>
      detail === null ? null : <InspectMetrics namespace={detail.namespace} pod={detail.name} />,
    );
  }

  const mounts = DOCK_SIDES.flatMap((side) =>
    panelsOn(placement, side)
      .filter((id) => enabledOf(id))
      .filter((id) => opened.includes(id))
      .map((id) => {
        const active = activeOn(side) === id;
        return (
          <PanelMount key={id} host={hosts[side]} active={active}>
            {bodyOf(id, active)}
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
        onActivate={activate('left')}
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
          onActivate={activate('bottom')}
          onMove={handleMove}
          hostRef={registerHost.bottom}
          emptyHint={hintFor('bottom')}
        />
      </div>
      <PanelHost
        side="right"
        tabs={tabsFor('right')}
        active={activeOn('right')}
        onActivate={activate('right')}
        onMove={handleMove}
        hostRef={registerHost.right}
        emptyHint={hintFor('right')}
      />
      {mounts}
    </div>
  );
}

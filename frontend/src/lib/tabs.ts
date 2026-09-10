import type { Tab } from '../store/clusters';
import { useActiveCluster, useTabs } from '../store/clusters';
import { closeCluster, openCluster } from './clusters';
import { useForwardsStore } from '../store/forwards';
import { useTerminalsStore } from '../store/terminals';
import { forgetCatalog } from '../store/catalog';
import { forgetFilters } from '../store/filters';
import { forgetForwards } from '../store/forwards';
import { forgetHealth } from '../store/clusterHealth';
import { forgetNamespace } from '../store/namespace';
import { forgetRecents } from '../store/recents';
import { forgetTerminals } from '../store/terminals';

export function forgetTab(cluster: string): void {
  forgetCatalog(cluster);
  forgetFilters(cluster);
  forgetForwards(cluster);
  forgetHealth(cluster);
  forgetNamespace(cluster);
  forgetRecents(cluster);
  forgetTerminals(cluster);
}

export function tabFor(tabs: Tab[], kubeconfig: string, file: string, context: string): Tab | null {
  for (const tab of tabs) {
    if (tab.context !== context) {
      continue;
    }
    if (tab.kubeconfig === kubeconfig || tab.kubeconfig === file) {
      return tab;
    }
  }
  return null;
}

export function contextOf(tabs: Tab[], cluster: string): string {
  for (const tab of tabs) {
    if (tab.id === cluster) {
      return tab.context;
    }
  }
  return '';
}

export function attachedTo(cluster: string): string[] {
  const held: string[] = [];
  const shells = useTerminalsStore.getState().byCluster[cluster]?.sessions ?? [];
  if (shells.length === 1) {
    held.push('1 shell');
  }
  if (shells.length > 1) {
    held.push(`${String(shells.length)} shells`);
  }
  const forwards = useForwardsStore.getState().byCluster[cluster] ?? [];
  if (forwards.length === 1) {
    held.push('1 port-forward');
  }
  if (forwards.length > 1) {
    held.push(`${String(forwards.length)} port-forwards`);
  }
  return held;
}

export function displayName(tabs: Tab[], cluster: string, fallback: string): string {
  for (const tab of tabs) {
    if (tab.id === cluster && tab.label !== '') {
      return tab.label;
    }
  }
  return fallback;
}

export function useShownCluster(): string {
  const tabs = useTabs();
  const active = useActiveCluster();
  return displayName(tabs, active, contextOf(tabs, active));
}

const ROOMY = 6;

export function tabWidth(open: number): string {
  if (open > ROOMY) {
    return 'max-w-32';
  }
  return 'max-w-56';
}

export function anchorOf(swatch: HTMLElement, strip: HTMLElement | null): number {
  const box = swatch.parentElement;
  if (strip === null || box === null) {
    return 0;
  }
  return box.getBoundingClientRect().left - strip.getBoundingClientRect().left;
}

export async function reopenTab(tab: Tab): Promise<void> {
  await closeCluster(tab.id);
  forgetTab(tab.id);
  await openCluster(tab.kubeconfig, tab.context);
}

import type { Tab } from '../store/clusters';
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

// The name a person put on the tab wins over the context it came from; the
// context name stays the identifier in the address bar.
export function displayName(tabs: Tab[], cluster: string, fallback: string): string {
  for (const tab of tabs) {
    if (tab.id === cluster && tab.label !== '') {
      return tab.label;
    }
  }
  return fallback;
}

// Past a handful of tabs the strip scrolls rather than growing the window, and
// each tab gives up label width first. The cap is legibility, not memory.
const ROOMY = 6;

export function tabWidth(open: number): string {
  if (open > ROOMY) {
    return 'max-w-32';
  }
  return 'max-w-56';
}

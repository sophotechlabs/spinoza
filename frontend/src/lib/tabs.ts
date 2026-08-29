import type { Tab } from '../store/clusters';
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

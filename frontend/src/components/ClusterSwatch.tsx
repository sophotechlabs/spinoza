import { colorVar } from '../lib/clusterColor';
import { useActiveTab } from '../store/clusters';

export default function ClusterSwatch() {
  const tab = useActiveTab();
  if (tab === null) {
    return null;
  }
  return (
    <span
      aria-label={`${tab.context} is colour ${String(tab.color)}`}
      style={{ backgroundColor: colorVar(tab.color) }}
      className="mr-1.5 h-2.5 w-2.5 shrink-0 rounded-sm"
    />
  );
}

import ClusterSwatch from './ClusterSwatch';
import { useActiveTab } from '../store/clusters';

export default function ClusterBadge() {
  const tab = useActiveTab();
  if (tab === null) {
    return null;
  }
  return (
    <span className="flex min-w-0 items-center text-fg-soft normal-case">
      <ClusterSwatch />
      <span className="truncate font-normal">on {tab.context}</span>
    </span>
  );
}

import type { ObjectRef, ReleaseRef } from '../lib/types';
import HelmReleaseDetail from './HelmReleaseDetail';

interface ReleasePanelProps {
  target: ReleaseRef | null;
  onSelectResource: (ref: ObjectRef) => void;
  onOpenResource: (ref: ObjectRef, kind: string) => void;
  onClose: () => void;
}

export default function ReleasePanel({
  target,
  onSelectResource,
  onOpenResource,
  onClose,
}: ReleasePanelProps) {
  if (target === null) {
    return <p className="p-4 text-xs text-fg-muted">Select a Helm release to inspect it.</p>;
  }
  return (
    <HelmReleaseDetail
      key={`${target.namespace}/${target.name}`}
      namespace={target.namespace}
      name={target.name}
      onSelectResource={onSelectResource}
      onOpenResource={onOpenResource}
      onClose={onClose}
    />
  );
}

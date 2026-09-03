import type { EntityDetail, EntityIdentity } from '../lib/entityLabel';
import { entityDetails } from '../lib/entityLabel';

interface EntityLabelProps extends EntityIdentity {
  detail: EntityDetail;
  className?: string;
}

export default function EntityLabel({
  name,
  kind,
  group,
  version,
  namespace,
  cluster,
  detail,
  className = '',
}: EntityLabelProps) {
  const secondary = entityDetails({ name, kind, group, version, namespace, cluster }, detail);
  return (
    <span className={`flex min-w-0 items-baseline gap-1.5 ${className}`}>
      <span className="min-w-0 truncate text-fg-strong">{name}</span>
      {secondary !== '' && (
        <span className="shrink-0 text-[10px] text-fg-muted">· {secondary}</span>
      )}
    </span>
  );
}

import type { DisabledActionReason } from '../lib/actionAvailability';

export default function DisabledActionReasons({ reasons }: { reasons: DisabledActionReason[] }) {
  const visible = reasons.filter((reason) => reason.reason !== null);
  if (visible.length === 0) {
    return null;
  }
  return (
    <ul
      aria-label="Unavailable actions"
      className="mt-1 flex flex-wrap gap-x-3 text-[11px] text-fg-muted"
    >
      {visible.map((reason) => (
        <li key={reason.id} id={reason.id}>
          {reason.label} unavailable: {reason.reason}
        </li>
      ))}
    </ul>
  );
}

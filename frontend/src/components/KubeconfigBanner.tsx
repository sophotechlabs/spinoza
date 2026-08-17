import { useUnreadableCurrent } from '../store/contexts';

export default function KubeconfigBanner() {
  const gone = useUnreadableCurrent();

  if (gone === null) {
    return null;
  }

  return (
    <div
      role="status"
      className="flex shrink-0 items-baseline gap-2 border-b border-warn-line bg-warn-tint/40 px-3 py-1.5 text-xs text-warn-strong"
    >
      <span className="shrink-0 font-semibold text-warn">
        The kubeconfig this cluster came from cannot be read.
      </span>
      <span className="min-w-0 flex-1 truncate" title={gone.error}>
        {gone.label}: {gone.error}
      </span>
      <span className="shrink-0 text-warn-muted">
        The live connection still works; reopening this context will not.
      </span>
    </div>
  );
}

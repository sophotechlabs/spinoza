interface StaleBannerProps {
  what: string;
  message: string;
  onRetry: () => void;
}

export default function StaleBanner({ what, message, onRetry }: StaleBannerProps) {
  return (
    <div
      role="status"
      className="flex shrink-0 items-baseline gap-2 border-b border-warn-line bg-warn-tint/40 px-3 py-1.5 text-xs text-warn-strong"
    >
      <span className="shrink-0 font-semibold text-warn">{what} stopped updating.</span>
      <span className="min-w-0 flex-1 truncate" title={message}>
        {message}
      </span>
      <button
        type="button"
        onClick={onRetry}
        className="shrink-0 rounded border border-warn-line-strong px-1.5 py-0.5 text-warn-strong hover:bg-warn-tint"
      >
        Retry
      </button>
    </div>
  );
}

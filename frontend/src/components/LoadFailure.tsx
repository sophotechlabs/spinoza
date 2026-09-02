interface LoadFailureProps {
  what: string;
  message: string;
  onRetry?: () => void;
}

export default function LoadFailure({ what, message, onRetry }: LoadFailureProps) {
  let retry = null;
  if (onRetry !== undefined) {
    retry = (
      <button
        type="button"
        onClick={onRetry}
        className="mt-2 rounded border border-error-line px-1.5 py-0.5 text-error-strong hover:bg-error-tint"
      >
        Retry
      </button>
    );
  }

  return (
    <div role="alert" className="flex h-full items-start justify-center p-6 text-xs">
      <div className="max-w-2xl rounded border border-error-line bg-error-tint/40 px-3 py-2">
        <div className="font-semibold text-error">{what} could not be loaded</div>
        <div className="mt-1 break-words text-error-strong">{message}</div>
        {retry}
      </div>
    </div>
  );
}

interface LoadFailureProps {
  what: string;
  message: string;
}

export default function LoadFailure({ what, message }: LoadFailureProps) {
  return (
    <div role="alert" className="flex h-full items-start justify-center p-6 text-xs">
      <div className="max-w-2xl rounded border border-error-line bg-error-tint/40 px-3 py-2">
        <div className="font-semibold text-error">{what} could not be loaded</div>
        <div className="mt-1 break-words text-error-strong">{message}</div>
      </div>
    </div>
  );
}

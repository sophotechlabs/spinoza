interface LoadFailureProps {
  what: string;
  message: string;
}

export default function LoadFailure({ what, message }: LoadFailureProps) {
  return (
    <div className="flex h-full items-start justify-center p-6 text-xs">
      <div className="max-w-2xl rounded border border-red-900 bg-red-950/40 px-3 py-2">
        <div className="font-semibold text-red-400">{what} could not be loaded</div>
        <div className="mt-1 break-words text-red-300">{message}</div>
      </div>
    </div>
  );
}

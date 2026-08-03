interface LoadWarningProps {
  message: string;
}

export default function LoadWarning({ message }: LoadWarningProps) {
  return (
    <div
      role="status"
      className="max-h-20 shrink-0 overflow-y-auto border-b border-amber-900 bg-amber-950/40 px-3 py-1.5 text-xs text-amber-300"
    >
      <span className="font-semibold text-amber-400">Partial data. </span>
      <span className="break-words">{message}</span>
    </div>
  );
}

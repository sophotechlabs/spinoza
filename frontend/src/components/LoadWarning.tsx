interface LoadWarningProps {
  message: string;
}

export default function LoadWarning({ message }: LoadWarningProps) {
  return (
    <div
      role="status"
      className="max-h-20 shrink-0 overflow-y-auto border-b border-warn-line bg-warn-tint/40 px-3 py-1.5 text-xs text-warn-strong"
    >
      <span className="font-semibold text-warn">Partial data. </span>
      <span className="break-words">{message}</span>
    </div>
  );
}

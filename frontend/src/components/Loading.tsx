import { SpinnerIcon } from './icons';

interface LoadingProps {
  what: string;
}

export default function Loading({ what }: LoadingProps) {
  return (
    <div
      role="status"
      className="flex h-full min-h-16 items-center justify-center gap-2 p-3 text-xs text-fg-muted"
    >
      <SpinnerIcon />
      <span>Loading {what}</span>
    </div>
  );
}

interface LoadingProps {
  what: string;
}

export default function Loading({ what }: LoadingProps) {
  return <div className="p-3 text-xs text-fg-muted">Loading {what}</div>;
}

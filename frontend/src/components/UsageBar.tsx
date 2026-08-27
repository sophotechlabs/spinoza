import { barColor } from '../lib/metrics';

interface UsageBarProps {
  percent: number;
  label: string;
  // What to write beside the bar. The bar is the proportion, so a caller with
  // something better to say than a percentage — how much of how much — says it
  // here instead.
  text?: string;
}

export default function UsageBar({ percent, label, text }: UsageBarProps) {
  const width = Math.min(100, Math.max(0, percent));
  return (
    <span className="flex items-center gap-1.5" title={label}>
      <span className="h-1.5 w-10 shrink-0 overflow-hidden rounded-sm bg-surface-active">
        <span className={`block h-full ${barColor(percent)}`} style={{ width: `${width}%` }} />
      </span>
      <span className="text-fg-muted">{text ?? `${percent}%`}</span>
    </span>
  );
}

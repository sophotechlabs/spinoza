import type { ReactNode } from 'react';
import { actionClass } from '../lib/actions';
import type { ActionSize, ActionTone } from '../lib/actions';

interface ActionProps {
  label: string;
  onClick: () => void;
  tone?: ActionTone;
  size?: ActionSize;
  disabled?: boolean;
  title?: string;
  describedBy?: string;
}

interface ActionGroupProps {
  label?: string;
  children: ReactNode;
}

export function Action({
  label,
  onClick,
  tone = 'plain',
  size = 'normal',
  disabled = false,
  title,
  describedBy,
}: ActionProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={title}
      aria-describedby={describedBy}
      className={actionClass(tone, size)}
    >
      {label}
    </button>
  );
}

export function ActionNote({ children }: { children: ReactNode }) {
  return <span className="text-fg-muted">{children}</span>;
}

export default function ActionGroup({ label, children }: ActionGroupProps) {
  return (
    <div role="group" aria-label={label} className="flex flex-wrap items-center gap-2">
      {children}
    </div>
  );
}

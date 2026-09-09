import { useState } from 'react';
import type { ReactNode } from 'react';
import { SpinnerIcon } from './icons';
import { WARNING_LIMIT, shortened } from '../lib/warningText';
import { useShellExplains } from '../lib/health';

export type Capability = 'loading' | 'empty' | 'partial' | 'stale' | 'failed';

interface CapabilityStateProps {
  state: Capability;
  what?: string;
  why?: string;
  onRetry?: () => void;
  children?: ReactNode;
}

function moreLabel(open: boolean): string {
  if (open) {
    return 'Show less';
  }
  return 'Show more';
}

function Waiting({ what }: { what: string }) {
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

function Empty({ why, children }: { why: string; children?: ReactNode }) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-2 p-6 text-xs text-fg-muted">
      <span>{why}</span>
      {children}
    </div>
  );
}

function Partial({ why }: { why: string }) {
  const [open, setOpen] = useState(false);
  const long = why.length > WARNING_LIMIT;
  let shown = why;
  if (long && !open) {
    shown = shortened(why);
  }

  return (
    <div
      role="status"
      className="max-h-32 shrink-0 overflow-y-auto border-b border-warn-line bg-warn-tint/40 px-3 py-1.5 text-xs text-warn-strong"
    >
      <span className="font-semibold text-warn">Partial data. </span>
      <span className="break-words">{shown}</span>
      {long && (
        <button
          type="button"
          onClick={() => {
            setOpen(!open);
          }}
          className="ml-1 cursor-pointer underline underline-offset-2 hover:text-warn"
        >
          {moreLabel(open)}
        </button>
      )}
    </div>
  );
}

function Stale({ what, why, onRetry }: { what: string; why: string; onRetry?: () => void }) {
  let retry = null;
  if (onRetry !== undefined) {
    retry = (
      <button
        type="button"
        onClick={onRetry}
        className="shrink-0 rounded border border-warn-line-strong px-1.5 py-0.5 text-warn-strong hover:bg-warn-tint"
      >
        Retry
      </button>
    );
  }

  return (
    <div
      role="status"
      className="flex shrink-0 items-baseline gap-2 border-b border-warn-line bg-warn-tint/40 px-3 py-1.5 text-xs text-warn-strong"
    >
      <span className="shrink-0 font-semibold text-warn">{what} stopped updating.</span>
      <span className="min-w-0 flex-1 truncate" title={why}>
        {why}
      </span>
      {retry}
    </div>
  );
}

function Failed({ what, why, onRetry }: { what: string; why: string; onRetry?: () => void }) {
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
        <div className="mt-1 break-words text-error-strong">{why}</div>
        {retry}
      </div>
    </div>
  );
}

export default function CapabilityState({
  state,
  what = '',
  why = '',
  onRetry,
  children,
}: CapabilityStateProps) {
  const explained = useShellExplains();

  if (state === 'loading') {
    return <Waiting what={what} />;
  }
  if (state === 'empty') {
    return <Empty why={why}>{children}</Empty>;
  }
  if (state === 'failed') {
    return <Failed what={what} why={why} onRetry={onRetry} />;
  }
  if (explained) {
    return null;
  }
  if (state === 'partial') {
    return <Partial why={why} />;
  }
  return <Stale what={what} why={why} onRetry={onRetry} />;
}

import { useEffect, useState } from 'react';
import type { DataEntry } from '../lib/types';
import { EyeIcon, EyeOffIcon } from './icons';
import CopyButton from './CopyButton';

interface DataEntriesProps {
  uid: string;
  entries: DataEntry[];
  masked: boolean;
}

const MASK = '••••••••••••';

function sizeOf(entry: DataEntry): string {
  if (entry.bytes === 1) {
    return '1 byte';
  }
  return `${String(entry.bytes)} bytes`;
}

const MOST_ROWS = 12;

function rowsFor(value: string): number {
  const lines = value.split('\n').length;
  if (lines > MOST_ROWS) {
    return MOST_ROWS;
  }
  return lines;
}

function hint(entry: DataEntry): string {
  if (entry.binary) {
    return `${sizeOf(entry)}, shown as base64`;
  }
  return sizeOf(entry);
}

export default function DataEntries({ uid, entries, masked }: DataEntriesProps) {
  const [shown, setShown] = useState<string[]>([]);

  useEffect(() => {
    setShown([]);
  }, [uid]);

  function toggle(key: string) {
    setShown((current) => {
      if (current.includes(key)) {
        return current.filter((name) => name !== key);
      }
      return [...current, key];
    });
  }

  return (
    <div className="flex flex-col gap-2">
      {entries.map((entry) => {
        const open = !masked || shown.includes(entry.key);
        return (
          <div key={entry.key} className="flex flex-col gap-0.5">
            <div className="flex items-baseline gap-2">
              <span className="text-fg">{entry.key}</span>
              <span className="text-[11px] text-fg-muted">{hint(entry)}</span>
            </div>
            <div className="flex items-start gap-1">
              {open && entry.value.includes('\n') ? (
                <textarea
                  readOnly
                  aria-label={entry.key}
                  value={entry.value}
                  rows={rowsFor(entry.value)}
                  className="min-w-0 flex-1 resize-y rounded border border-edge bg-surface-raised px-2 py-1 font-mono text-fg-soft"
                />
              ) : (
                <input
                  readOnly
                  aria-label={entry.key}
                  value={open ? entry.value : MASK}
                  className="min-w-0 flex-1 rounded border border-edge bg-surface-raised px-2 py-1 font-mono text-fg-soft"
                />
              )}
              {masked && (
                <button
                  type="button"
                  aria-label={open ? `Hide ${entry.key}` : `Show ${entry.key}`}
                  title={open ? 'Hide the value' : 'Show the value'}
                  onClick={() => {
                    toggle(entry.key);
                  }}
                  className="shrink-0 rounded border border-edge-strong px-1 leading-none text-fg-muted hover:bg-surface-active"
                >
                  {open ? <EyeOffIcon /> : <EyeIcon />}
                </button>
              )}
              <CopyButton what={entry.key} text={entry.value} />
            </div>
          </div>
        );
      })}
    </div>
  );
}

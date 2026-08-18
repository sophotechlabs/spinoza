import { useState } from 'react';
import { WARNING_LIMIT, shortened } from '../lib/warningText';

interface LoadWarningProps {
  message: string;
}

function moreLabel(open: boolean): string {
  if (open) {
    return 'Show less';
  }
  return 'Show more';
}

export default function LoadWarning({ message }: LoadWarningProps) {
  const [open, setOpen] = useState(false);
  const long = message.length > WARNING_LIMIT;
  let shown = message;
  if (long && !open) {
    shown = shortened(message);
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

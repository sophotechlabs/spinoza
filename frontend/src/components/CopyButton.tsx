import { copyText } from '../lib/clipboard';

interface CopyButtonProps {
  what: string;
  text: string;
  quiet?: boolean;
}

function buttonClass(quiet: boolean): string {
  const base = 'shrink-0 rounded px-1 leading-none text-fg-muted hover:bg-surface-active';
  if (quiet) {
    return `${base} opacity-0 group-hover:opacity-100 focus:opacity-100`;
  }
  return `${base} border border-edge-strong`;
}

export default function CopyButton({ what, text, quiet = false }: CopyButtonProps) {
  return (
    <button
      type="button"
      aria-label={`Copy ${what}`}
      title={`Copy ${what}`}
      onClick={() => void copyText(what, text)}
      className={buttonClass(quiet)}
    >
      ⧉
    </button>
  );
}

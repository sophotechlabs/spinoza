import { useEffect } from 'react';
import type { ToastTone } from '../store/toasts';
import { useToastsStore } from '../store/toasts';

export const TOAST_TTL_MS = 6000;

function toneClass(tone: ToastTone): string {
  if (tone === 'error') {
    return 'rounded border border-error-line bg-error-tint px-2 py-1.5 text-error-strong';
  }
  if (tone === 'warn') {
    return 'rounded border border-warn-line bg-warn-tint px-2 py-1.5 text-warn-strong';
  }
  return 'rounded border border-ok-line bg-ok-tint px-2 py-1.5 text-fg';
}

export default function Toasts() {
  const toasts = useToastsStore((state) => state.toasts);
  const dismiss = useToastsStore((state) => state.dismiss);

  useEffect(() => {
    if (toasts.length === 0) {
      return;
    }
    const oldest = toasts[0];
    const timer = setTimeout(() => {
      dismiss(oldest.id);
    }, TOAST_TTL_MS);
    return () => {
      clearTimeout(timer);
    };
  }, [toasts, dismiss]);

  return (
    <div
      aria-live="polite"
      aria-label="Notifications"
      className="pointer-events-none fixed right-3 bottom-3 z-50 flex w-96 flex-col gap-1.5 text-xs"
    >
      {toasts.map((toast) => (
        <div key={toast.id} className={`pointer-events-auto flex gap-2 ${toneClass(toast.tone)}`}>
          <span className="min-w-0 flex-1 break-words">{toast.message}</span>
          <button
            type="button"
            aria-label={`Dismiss: ${toast.message}`}
            onClick={() => {
              dismiss(toast.id);
            }}
            className="shrink-0 text-fg-muted hover:text-fg"
          >
            ✕
          </button>
        </div>
      ))}
    </div>
  );
}

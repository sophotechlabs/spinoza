interface MovedToDesktopProps {
  open: boolean;
  onStay: () => void;
}

export default function MovedToDesktop({ open, onStay }: MovedToDesktopProps) {
  if (!open) {
    return null;
  }

  return (
    <div
      role="status"
      className="fixed inset-0 z-50 flex items-center justify-center bg-surface/95 text-xs"
    >
      <div className="w-96 rounded border border-edge-strong bg-surface-raised p-4">
        <p className="font-semibold text-fg-strong">Spinoza is back in its window</p>
        <p className="mt-2 text-fg-soft">You can close this tab.</p>
        <button
          type="button"
          onClick={onStay}
          className="mt-3 rounded border border-edge-strong px-2 py-1 text-fg hover:bg-surface-active"
        >
          Stay here
        </button>
      </div>
    </div>
  );
}

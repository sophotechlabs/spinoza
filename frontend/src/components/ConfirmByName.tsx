import { useEffect, useRef, useState } from 'react';

interface ConfirmByNameProps {
  open: boolean;
  name: string;
  what: string;
  onConfirm: () => void;
  onCancel: () => void;
}

export default function ConfirmByName({
  open,
  name,
  what,
  onConfirm,
  onCancel,
}: ConfirmByNameProps) {
  const ref = useRef<HTMLDialogElement | null>(null);
  const [typed, setTyped] = useState('');

  useEffect(() => {
    const dialog = ref.current;
    if (open && dialog?.open === false) {
      setTyped('');
      dialog.showModal();
    }
    if (!open && dialog?.open === true) {
      dialog.close();
    }
  }, [open]);

  const matches = typed === name;

  return (
    <dialog
      ref={ref}
      aria-label="Confirm on a protected cluster"
      onClose={onCancel}
      className="backdrop:bg-black/50 m-auto w-[28rem] rounded border border-warn-line bg-surface p-0 text-fg"
    >
      <div className="border-b border-edge px-3 py-2 text-xs font-semibold tracking-wide text-warn uppercase">
        This cluster is protected
      </div>
      <div className="p-3 text-xs">
        <p className="text-fg-soft">{what}</p>
        <p className="mt-2 text-fg-soft">
          Type <span className="font-semibold text-fg-strong">{name}</span> to go ahead.
        </p>
        <label htmlFor="confirm-name" className="mt-3 block text-fg">
          Name
        </label>
        <input
          id="confirm-name"
          type="text"
          value={typed}
          spellCheck={false}
          autoComplete="off"
          placeholder={name}
          onChange={(event) => {
            setTyped(event.target.value);
          }}
          className="mt-1 w-full rounded border border-edge-strong bg-surface-raised px-2 py-1 font-mono text-fg"
        />
        <div className="mt-3 flex items-center justify-end gap-2">
          <button
            type="button"
            onClick={onCancel}
            className="rounded border border-edge-strong px-2 py-1 text-fg-soft hover:bg-surface-active"
          >
            Cancel
          </button>
          <button
            type="button"
            disabled={!matches}
            onClick={onConfirm}
            className="rounded border border-error-line-strong px-2 py-1 text-error-contrast hover:bg-error-tint-strong disabled:border-edge-strong disabled:text-fg-subtle"
          >
            Confirm
          </button>
        </div>
      </div>
    </dialog>
  );
}

import { useEffect, useRef, useState } from 'react';
import type { ContextList, Kubeconfig } from '../lib/types';
import {
  addKubeconfig,
  fetchFilePicker,
  pickKubeconfigFile,
  removeKubeconfig,
} from '../lib/contexts';
import { notifyError, notifyOk } from '../store/toasts';

interface KubeconfigDialogProps {
  open: boolean;
  kubeconfigs: Kubeconfig[];
  onClose: () => void;
  onChanged: (list: ContextList) => void;
}

function reason(err: unknown, fallback: string): string {
  if (err instanceof Error) {
    return err.message;
  }
  return fallback;
}

function contextCount(entry: Kubeconfig): string {
  if (entry.contexts.length === 1) {
    return '1 context';
  }
  return `${entry.contexts.length} contexts`;
}

export default function KubeconfigDialog({
  open,
  kubeconfigs,
  onClose,
  onChanged,
}: KubeconfigDialogProps) {
  const ref = useRef<HTMLDialogElement | null>(null);
  const [path, setPath] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [browsable, setBrowsable] = useState(false);

  useEffect(() => {
    const dialog = ref.current;
    if (open && dialog?.open === false) {
      dialog.showModal();
    }
    if (!open && dialog?.open === true) {
      dialog.close();
    }
  }, [open]);

  useEffect(() => {
    if (!open) {
      return;
    }
    let live = true;
    fetchFilePicker()
      .then((support) => {
        if (live) {
          setBrowsable(support.available);
        }
      })
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [open]);

  async function run(work: () => Promise<ContextList>, failed: string, done: string) {
    setBusy(true);
    setError(null);
    try {
      onChanged(await work());
      notifyOk(done);
    } catch (err: unknown) {
      const message = reason(err, failed);
      setError(message);
      notifyError(`${failed}: ${message}`);
    } finally {
      setBusy(false);
    }
  }

  async function handleAdd() {
    const wanted = path.trim();
    if (wanted === '') {
      setError('type the path of a kubeconfig file');
      return;
    }
    await run(() => addKubeconfig(wanted), 'Adding that kubeconfig', `Reading ${wanted}`);
    setPath('');
  }

  async function handleRemove(entry: Kubeconfig) {
    await run(
      () => removeKubeconfig(entry.path),
      'Removing that kubeconfig',
      `Stopped reading ${entry.label}`,
    );
  }

  async function handleBrowse() {
    setError(null);
    try {
      const chosen = await pickKubeconfigFile();
      if (chosen !== '') {
        setPath(chosen);
      }
    } catch (err: unknown) {
      setError(reason(err, 'the file dialog did not open'));
    }
  }

  return (
    <dialog
      ref={ref}
      aria-label="Kubeconfigs"
      onClose={onClose}
      className="backdrop:bg-black/50 m-auto w-[36rem] rounded border border-edge-strong bg-surface p-0 text-fg"
    >
      <div className="flex items-center justify-between border-b border-edge px-3 py-2">
        <h2 className="text-xs font-semibold tracking-wide text-fg-strong uppercase">
          Kubeconfigs
        </h2>
        <button
          type="button"
          onClick={onClose}
          className="rounded border border-edge-strong px-2 py-0.5 text-xs text-fg-soft hover:bg-surface-active"
        >
          Close
        </button>
      </div>
      <div className="p-3 text-xs">
        <p className="text-fg-muted">
          Spinoza reads these files every time it lists or opens a cluster. Nothing is copied, and
          they are never merged with each other.
        </p>
        <ul className="mt-3 space-y-2">
          {kubeconfigs.map((entry) => (
            <li key={entry.path} className="flex items-start gap-2 border-b border-edge pb-2">
              <span className="min-w-0 flex-1">
                <span className="block truncate text-fg">{entry.label}</span>
                <span className="text-fg-muted">
                  {entry.path === '' && <span>read by default, </span>}
                  {contextCount(entry)}
                </span>
                {entry.error !== undefined && (
                  <span className="mt-0.5 block text-error">{entry.error}</span>
                )}
              </span>
              {entry.removable && (
                <button
                  type="button"
                  aria-label={`Remove ${entry.label}`}
                  disabled={busy}
                  onClick={() => void handleRemove(entry)}
                  className="rounded border border-error-line px-1.5 py-0.5 text-error hover:bg-error-tint disabled:text-fg-subtle"
                >
                  Remove
                </button>
              )}
            </li>
          ))}
        </ul>
        <div className="mt-3">
          <label htmlFor="kubeconfig-path" className="text-fg">
            Add a kubeconfig
          </label>
          <div className="mt-1 flex items-center gap-2">
            <input
              id="kubeconfig-path"
              type="text"
              value={path}
              spellCheck={false}
              placeholder="~/.kube/other.yaml"
              onChange={(event) => {
                setPath(event.target.value);
              }}
              className="min-w-0 flex-1 rounded border border-edge-strong bg-surface-raised px-2 py-1 font-mono text-fg"
            />
            {browsable && (
              <button
                type="button"
                onClick={() => void handleBrowse()}
                className="rounded border border-edge-strong px-2 py-1 text-fg hover:bg-surface-active"
              >
                Browse
              </button>
            )}
            <button
              type="button"
              disabled={busy}
              onClick={() => void handleAdd()}
              className="rounded border border-edge-strong px-2 py-1 text-fg hover:bg-surface-active disabled:text-fg-subtle"
            >
              Add
            </button>
          </div>
          {error !== null && (
            <p role="status" className="mt-1 text-error">
              {error}
            </p>
          )}
        </div>
      </div>
    </dialog>
  );
}

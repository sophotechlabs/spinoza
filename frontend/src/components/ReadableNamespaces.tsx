import { useState } from 'react';
import { parseNamespaceList, readNamespaces, writeNamespaces } from '../lib/settings';
import { useNamespaceStore } from '../store/namespace';

interface ReadableNamespacesProps {
  context: string;
}

function wrongNote(wrong: string[]): string {
  if (wrong.length === 1) {
    return `${wrong[0]} is not a namespace name.`;
  }
  return `${wrong.join(', ')} are not namespace names.`;
}

function hintFor(context: string): string {
  if (context === '') {
    return 'For an account that cannot list namespaces. Spinoza also tries the namespace set in the kubeconfig.';
  }
  return `For when ${context} cannot list namespaces. Spinoza also tries the namespace set in the kubeconfig, and shows only the ones this account can read.`;
}

export default function ReadableNamespaces({ context }: ReadableNamespacesProps) {
  const askAgain = useNamespaceStore((state) => state.askAgain);
  const [draft, setDraft] = useState(() => (readNamespaces()[context] ?? []).join(', '));
  const [wrong, setWrong] = useState<string[]>([]);
  const [saved, setSaved] = useState(false);

  function save() {
    const parsed = parseNamespaceList(draft);
    setWrong(parsed.wrong);
    setSaved(false);
    if (parsed.wrong.length > 0) {
      return;
    }
    setDraft(parsed.names.join(', '));
    void writeNamespaces({ ...readNamespaces(), [context]: parsed.names }).then(() => {
      setSaved(true);
      askAgain();
    });
  }

  return (
    <div className="border-b border-edge px-1 py-3 last:border-b-0">
      <form
        className="flex items-center justify-between gap-4"
        onSubmit={(event) => {
          event.preventDefault();
          save();
        }}
      >
        <label htmlFor="readable-namespaces" className="text-fg">
          Namespaces you can read
        </label>
        <span className="flex items-center gap-2">
          <input
            id="readable-namespaces"
            type="text"
            value={draft}
            disabled={context === ''}
            placeholder="payments, storefront"
            onChange={(event) => {
              setDraft(event.target.value);
              setSaved(false);
            }}
            className="w-48 rounded border border-edge-strong bg-surface-raised px-2 py-0.5 text-fg"
          />
          <button
            type="submit"
            disabled={context === ''}
            className="rounded border border-edge-strong px-2 py-0.5 text-fg hover:bg-surface-active disabled:cursor-not-allowed disabled:text-fg-muted"
          >
            Save
          </button>
        </span>
      </form>
      <p className="mt-1 text-[11px] text-fg-muted">{hintFor(context)}</p>
      {wrong.length > 0 && (
        <p role="alert" className="mt-1 text-[11px] text-error">
          {wrongNote(wrong)}
        </p>
      )}
      {saved && (
        <p role="status" className="mt-1 text-[11px] text-fg-muted">
          Saved.
        </p>
      )}
    </div>
  );
}

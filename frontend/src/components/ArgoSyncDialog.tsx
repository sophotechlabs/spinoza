import { useEffect, useRef, useState } from 'react';
import type { ArgoOptions, ArgoResourceRef } from '../lib/argoActions';
import ClusterBadge from './ClusterBadge';

interface ArgoSyncDialogProps {
  name: string;
  resources?: ArgoResourceRef[];
  onRun: (options: ArgoOptions) => void;
  onCancel: () => void;
}

interface Choices {
  prune: boolean;
  dryRun: boolean;
  applyOnly: boolean;
  force: boolean;
  replace: boolean;
  serverSide: boolean;
}

interface Field {
  key: keyof Choices;
  label: string;
  hint: string;
}

const FIELDS: Field[] = [
  { key: 'prune', label: 'Prune', hint: 'Delete what git no longer declares' },
  { key: 'dryRun', label: 'Dry run', hint: 'Report what would change, write nothing' },
  { key: 'applyOnly', label: 'Apply only', hint: 'Skip the PreSync and PostSync hooks' },
  { key: 'force', label: 'Force', hint: 'Recreate a resource that refuses to patch' },
  { key: 'replace', label: 'Replace', hint: 'Replace each resource instead of applying to it' },
  { key: 'serverSide', label: 'Server-side apply', hint: 'Let the api server merge the fields' },
];

const NOTHING: Choices = {
  prune: false,
  dryRun: false,
  applyOnly: false,
  force: false,
  replace: false,
  serverSide: false,
};

const buttonClass =
  'rounded border border-edge-strong px-2 py-1 text-fg-soft hover:bg-surface-active disabled:cursor-not-allowed disabled:text-fg-faint';

const primaryClass =
  'rounded border border-warn-line px-2 py-1 text-warn hover:bg-warn-tint disabled:cursor-not-allowed disabled:border-edge disabled:text-fg-faint';

function scope(resources: ArgoResourceRef[] | undefined): string {
  if (resources === undefined || resources.length === 0) {
    return 'every resource this application manages';
  }
  if (resources.length === 1) {
    return `${resources[0].kind} ${resources[0].name}`;
  }
  return `${String(resources.length)} marked resources`;
}

export default function ArgoSyncDialog({ name, resources, onRun, onCancel }: ArgoSyncDialogProps) {
  const ref = useRef<HTMLDialogElement | null>(null);
  const [choices, setChoices] = useState<Choices>(NOTHING);

  useEffect(() => {
    const dialog = ref.current;
    if (dialog?.open === false) {
      dialog.showModal();
    }
  }, []);

  function toggle(key: keyof Choices) {
    setChoices((current) => ({ ...current, [key]: !current[key] }));
  }

  function run() {
    onRun({ ...choices, resources });
  }

  const hooksKept = choices.force && !choices.applyOnly;

  return (
    <dialog
      ref={ref}
      aria-label={`Sync ${name}`}
      onClose={onCancel}
      className="backdrop:bg-black/50 m-auto w-[30rem] rounded border border-edge-strong bg-surface p-0 text-fg"
    >
      <div className="flex items-center gap-2 border-b border-edge px-3 py-2 text-xs font-semibold tracking-wide text-fg-strong uppercase">
        Sync {name}
        <ClusterBadge />
      </div>
      <div className="p-3 text-xs">
        <p className="text-fg-soft">Syncing {scope(resources)}.</p>
        <ul className="mt-3 space-y-2">
          {FIELDS.map((field) => (
            <li key={field.key} className="flex items-baseline gap-2">
              <input
                id={`argo-sync-${field.key}`}
                type="checkbox"
                checked={choices[field.key]}
                onChange={() => {
                  toggle(field.key);
                }}
              />
              <label htmlFor={`argo-sync-${field.key}`} className="min-w-0">
                <span className="text-fg-strong">{field.label}</span>
                <span className="ml-2 text-fg-muted">{field.hint}</span>
              </label>
            </li>
          ))}
        </ul>
        {hooksKept && (
          <p className="mt-3 text-fg-muted">The PreSync and PostSync hooks still run.</p>
        )}
        <div className="mt-3 flex items-center justify-end gap-2">
          <button type="button" onClick={onCancel} className={buttonClass}>
            Cancel
          </button>
          <button type="button" onClick={run} className={primaryClass}>
            Synchronize
          </button>
        </div>
      </div>
    </dialog>
  );
}

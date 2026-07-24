import type { PodRow } from '../lib/types';

interface DetailsDrawerProps {
  pod: PodRow | null;
  onClose?: () => void;
}

export default function DetailsDrawer({ pod, onClose }: DetailsDrawerProps) {
  if (!pod) {
    return (
      <aside className="w-80 shrink-0 border-l border-neutral-800 bg-neutral-950 p-4 text-xs text-neutral-500">
        Select a pod to see details.
      </aside>
    );
  }

  function handleClose() {
    if (onClose) {
      onClose();
    }
  }

  const fields: Array<[string, string]> = [
    ['Name', pod.name],
    ['Namespace', pod.namespace],
    ['Status', pod.phase],
    ['Ready', pod.ready],
    ['Restarts', String(pod.restarts)],
    ['Node', pod.node],
    ['Created', pod.createdAt],
    ['UID', pod.uid],
  ];

  return (
    <aside className="w-80 shrink-0 border-l border-neutral-800 bg-neutral-950 overflow-y-auto text-xs">
      <div className="flex items-center justify-between border-b border-neutral-800 px-4 py-2">
        <span className="truncate font-semibold text-neutral-100">{pod.name}</span>
        <button
          type="button"
          onClick={handleClose}
          className="ml-2 rounded border border-neutral-700 px-1.5 text-neutral-300 hover:bg-neutral-800"
        >
          Close
        </button>
      </div>
      <dl className="p-4">
        {fields.map(([label, value]) => (
          <div key={label} className="mb-2">
            <dt className="text-[11px] uppercase tracking-wide text-neutral-500">{label}</dt>
            <dd className="break-all text-neutral-200">{value}</dd>
          </div>
        ))}
      </dl>
    </aside>
  );
}

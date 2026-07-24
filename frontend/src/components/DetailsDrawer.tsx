import type { Row } from '../lib/types';

interface DetailsDrawerProps {
  row: Row | null;
  onClose?: () => void;
}

export default function DetailsDrawer({ row, onClose }: DetailsDrawerProps) {
  if (!row) {
    return (
      <aside className="w-80 shrink-0 border-l border-neutral-800 bg-neutral-950 p-4 text-xs text-neutral-500">
        Select a row to see details.
      </aside>
    );
  }

  function handleClose() {
    if (onClose) {
      onClose();
    }
  }

  const fields: [string, string][] = [
    ['Name', row.name],
    ['Namespace', row.namespace],
    ['Created', row.createdAt],
    ['UID', row.uid],
  ];

  return (
    <aside className="w-80 shrink-0 overflow-y-auto border-l border-neutral-800 bg-neutral-950 text-xs">
      <div className="flex items-center justify-between border-b border-neutral-800 px-4 py-2">
        <span className="truncate font-semibold text-neutral-100">{row.name}</span>
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
            <dt className="text-[11px] tracking-wide text-neutral-500 uppercase">{label}</dt>
            <dd className="break-all text-neutral-200">{value}</dd>
          </div>
        ))}
      </dl>
    </aside>
  );
}

const inactiveGroups = ['Config', 'Network', 'Storage', 'Access Control', 'Custom Resources'];

export default function Sidebar() {
  return (
    <nav className="w-56 shrink-0 overflow-y-auto border-r border-neutral-800 bg-neutral-950 py-2">
      <div className="mb-2">
        <div className="px-3 py-1 text-[11px] font-semibold tracking-wide text-neutral-400 uppercase">
          Workloads
        </div>
        <div className="mx-1 rounded bg-neutral-800 px-3 py-1 text-neutral-100">Pods</div>
      </div>
      {inactiveGroups.map((group) => (
        <div
          key={group}
          className="px-3 py-1 text-[11px] font-semibold tracking-wide text-neutral-600 uppercase"
        >
          {group}
        </div>
      ))}
    </nav>
  );
}

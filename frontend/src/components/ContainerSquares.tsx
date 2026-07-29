import type { Row } from '../lib/types';
import { containerColor, containerTitle } from '../lib/status';
import { isDebugContainer } from '../lib/containers';

interface ContainerSquaresProps {
  row: Row;
  fallback: string;
}

export default function ContainerSquares({ row, fallback }: ContainerSquaresProps) {
  const containers = (row.containers ?? []).filter((container) => !isDebugContainer(container));
  if (containers.length === 0) {
    return <span className="text-neutral-400">{fallback}</span>;
  }
  return (
    <span className="flex flex-wrap items-center gap-0.5">
      {containers.map((container) => (
        <span
          key={container.name}
          title={containerTitle(container)}
          className={`inline-block h-2.5 w-2.5 rounded-[2px] ${containerColor(container)}`}
        />
      ))}
    </span>
  );
}

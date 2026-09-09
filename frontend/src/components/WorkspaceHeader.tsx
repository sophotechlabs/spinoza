import type { ReactNode } from 'react';
import { ROW } from '../lib/rows';

interface WorkspaceHeaderProps {
  title: string;
  scope?: string;
  scale?: string;
  stale?: string;
  children?: ReactNode;
}

function Separator() {
  return (
    <span aria-hidden="true" className="text-fg-faint">
      ·
    </span>
  );
}

export default function WorkspaceHeader({
  title,
  scope,
  scale,
  stale,
  children,
}: WorkspaceHeaderProps) {
  let said = null;
  if (scope !== undefined && scope !== '') {
    said = (
      <>
        <Separator />
        <span className="min-w-0 truncate text-fg-muted">{scope}</span>
      </>
    );
  }

  let counted = null;
  if (scale !== undefined && scale !== '') {
    counted = (
      <>
        <Separator />
        <span className="shrink-0 whitespace-nowrap text-fg-muted">{scale}</span>
      </>
    );
  }

  let stamp = null;
  if (stale !== undefined && stale !== '') {
    stamp = (
      <>
        <Separator />
        <span className="shrink-0 whitespace-nowrap text-warn">as of {stale}</span>
      </>
    );
  }

  return (
    <div className={ROW}>
      <h2 className="shrink-0 font-semibold whitespace-nowrap text-fg-strong">{title}</h2>
      {said}
      {counted}
      {stamp}
      {children}
    </div>
  );
}

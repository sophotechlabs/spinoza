import { useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import type { ReactNode } from 'react';
import ErrorBoundary from './ErrorBoundary';

interface PanelMountProps {
  host: HTMLElement | null;
  active: boolean;
  label: string;
  id: string;
  labelledBy: string;
  children: ReactNode;
}

export default function PanelMount({
  host,
  active,
  label,
  id,
  labelledBy,
  children,
}: PanelMountProps) {
  const [node] = useState(() => document.createElement('div'));
  const hostRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    node.className = 'min-h-0 min-w-0 flex-1 flex-col';
    node.setAttribute('role', 'tabpanel');
    node.id = id;
    node.setAttribute('aria-labelledby', labelledBy);
    if (host === null) {
      return;
    }
    host.appendChild(node);
    hostRef.current = host;
    return () => {
      node.remove();
      hostRef.current = null;
    };
  }, [host, node, id, labelledBy]);

  useEffect(() => {
    if (active) {
      node.style.display = 'flex';
      node.removeAttribute('hidden');
      node.tabIndex = 0;
      return;
    }
    node.style.display = 'none';
    node.setAttribute('hidden', '');
    node.removeAttribute('tabindex');
  }, [active, node]);

  return createPortal(<ErrorBoundary label={label}>{children}</ErrorBoundary>, node);
}

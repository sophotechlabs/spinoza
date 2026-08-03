import { useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import type { ReactNode } from 'react';

interface PanelMountProps {
  host: HTMLElement | null;
  active: boolean;
  children: ReactNode;
}

export default function PanelMount({ host, active, children }: PanelMountProps) {
  const [node] = useState(() => document.createElement('div'));
  const hostRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    node.className = 'min-h-0 min-w-0 flex-1 flex-col';
    if (host === null) {
      return;
    }
    host.appendChild(node);
    hostRef.current = host;
    return () => {
      node.remove();
      hostRef.current = null;
    };
  }, [host, node]);

  useEffect(() => {
    if (active) {
      node.style.display = 'flex';
      return;
    }
    node.style.display = 'none';
  }, [active, node]);

  return createPortal(children, node);
}
